package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"cdr.dev/slog"
	"github.com/julienschmidt/httprouter"

	"github.com/tatsuworks/gateway/internal/state"
)

const (
	// voiceReadinessMaxAge rejects readiness the shard has not refreshed
	// recently. The gateway heartbeats every 5 seconds.
	voiceReadinessMaxAge = 15 * time.Second
	voiceTimeLayout      = "2006-01-02T15:04:05.000000Z"
)

type voiceOccupantJSON struct {
	DiscordUserID    string `json:"discordUserId"`
	ChannelID        string `json:"channelId"`
	IsBot            *bool  `json:"isBot"`
	LastChangedAtUtc string `json:"lastChangedAtUtc"`
	DiscordSequence  string `json:"discordSequence"`
}

type voiceLossJSON struct {
	Revision        string `json:"revision"`
	KnownThroughUtc string `json:"knownThroughUtc"`
	DetectedAtUtc   string `json:"detectedAtUtc"`
	Reason          string `json:"reason"`
}

// voiceStateJSON is the voice attendance read. IDs, sequences and revisions are
// decimal strings so no consumer has to trust a float with a snowflake.
type voiceStateJSON struct {
	GuildID       string              `json:"guildId"`
	ShardID       *int                `json:"shardId"`
	ConnectionID  string              `json:"connectionId"`
	ObservedAtUtc string              `json:"observedAtUtc"`
	Ready         bool                `json:"ready"`
	Complete      bool                `json:"complete"`
	Reason        string              `json:"reason,omitempty"`
	Occupants     []voiceOccupantJSON `json:"occupants"`
	Loss          *voiceLossJSON      `json:"loss"`
}

// getGuildVoiceState returns a guild's current voice presence with the
// evidence that vouches for it. Anything short of a ready, complete baseline
// is 503: an unavailable source must never look like an empty channel. The
// 503 body still carries the shard's loss, so a consumer can tell how far its
// earlier observations remain good. afterLossRevision limits the loss to ones
// the caller has not seen.
func (s *Server) getGuildVoiceState(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return err
	}
	var after int64
	if v := r.URL.Query().Get("afterLossRevision"); v != "" {
		after, err = strconv.ParseInt(v, 10, 64)
		if err != nil || after < 0 {
			return ErrInvalidArgument
		}
	}

	snap, err := s.db.GetGuildVoiceSnapshot(r.Context(), guild, after, voiceReadinessMaxAge)
	if err != nil {
		s.log.Error(r.Context(), "read guild voice state", slog.Error(err), slog.F("guild", guild))
		return writeVoiceState(w, http.StatusServiceUnavailable, voiceStateJSON{
			GuildID:   strconv.FormatInt(guild, 10),
			Reason:    "state_unavailable",
			Occupants: []voiceOccupantJSON{},
		})
	}

	body := voiceStateBody(snap)
	status := http.StatusOK
	if !snap.Ready || !snap.Complete {
		status = http.StatusServiceUnavailable
		if body.Reason == "" {
			body.Reason = "unclassified_occupant"
		}
		body.Occupants = []voiceOccupantJSON{}
	}
	return writeVoiceState(w, status, body)
}

func voiceStateBody(snap *state.VoiceSnapshot) voiceStateJSON {
	body := voiceStateJSON{
		GuildID:       strconv.FormatInt(snap.GuildID, 10),
		ConnectionID:  snap.ConnectionID,
		ObservedAtUtc: snap.ObservedAt.UTC().Format(voiceTimeLayout),
		Ready:         snap.Ready,
		Complete:      snap.Complete,
		Reason:        snap.NotReadyReason,
		Occupants:     make([]voiceOccupantJSON, 0, len(snap.Occupants)),
	}
	if snap.ShardID >= 0 {
		shard := snap.ShardID
		body.ShardID = &shard
	}
	for _, o := range snap.Occupants {
		body.Occupants = append(body.Occupants, voiceOccupantJSON{
			DiscordUserID:    strconv.FormatInt(o.UserID, 10),
			ChannelID:        strconv.FormatInt(o.ChannelID, 10),
			IsBot:            o.IsBot,
			LastChangedAtUtc: o.ChangedAt.UTC().Format(voiceTimeLayout),
			DiscordSequence:  strconv.FormatInt(o.DiscordSeq, 10),
		})
	}
	if snap.Loss != nil {
		body.Loss = &voiceLossJSON{
			Revision:        strconv.FormatInt(snap.Loss.Revision, 10),
			KnownThroughUtc: snap.Loss.KnownThrough.UTC().Format(voiceTimeLayout),
			DetectedAtUtc:   snap.Loss.DetectedAt.UTC().Format(voiceTimeLayout),
			Reason:          snap.Loss.Reason,
		}
	}
	return body
}

func writeVoiceState(w http.ResponseWriter, status int, body voiceStateJSON) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(body)
}
