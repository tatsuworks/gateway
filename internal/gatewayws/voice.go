package gatewayws

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	jsoniter "github.com/json-iterator/go"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/internal/state"
	"github.com/tatsuworks/gateway/internal/state/db/statepsql"
)

// Voice attendance support (TATSU-3540).
//
// For the guilds in VOICE_ATTENDANCE_GUILDS the gateway keeps State's voice
// presence, heartbeats per-shard readiness, stamps each VOICE_STATE_UPDATE with
// receipt metadata for the Redis destinations in RECEIPT_METADATA_REDIS, and
// records real losses of voice observation. Every other guild, destination and
// shard behaves exactly as before.

const (
	// VoiceLossEvent is the queue name of the best-effort loss notification.
	// It is pushed only to destinations whose whitelist names it explicitly, so
	// no consumer that does not read it ever receives it.
	VoiceLossEvent = "ATTENDANCE_VOICE_LOSS"

	voiceHeartbeatInterval = 5 * time.Second
	voiceDBTimeout         = 2 * time.Second
	receiptTimeLayout      = "2006-01-02T15:04:05.000000Z"
)

// attendanceGuilds is the hard bound for voice attendance: Wanderer Tech
// (staging) and Tatsu's Lounge (production). VOICE_ATTENDANCE_GUILDS may
// narrow this set but never widen it.
var attendanceGuilds = map[int64]struct{}{
	1347103391402164315: {},
	173184118492889089:  {},
}

type voiceConfig struct {
	guilds        map[int64]struct{}
	metadataRedis map[string]struct{}
	// reidentify asks for a fresh baseline after State's voice cache is known
	// to be wrong (a failed write or a cache reset), through the normal paced
	// identify path.
	reidentify bool
}

func parseVoiceConfig(guilds, metadataRedis, reidentify string) voiceConfig {
	cfg := voiceConfig{
		guilds:        map[int64]struct{}{},
		metadataRedis: map[string]struct{}{},
		reidentify:    reidentify != "false",
	}
	for _, field := range strings.Split(guilds, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
		if _, allowed := attendanceGuilds[id]; err == nil && allowed {
			cfg.guilds[id] = struct{}{}
		}
	}
	for _, field := range strings.Split(metadataRedis, ",") {
		if addr := strings.TrimSpace(field); addr != "" {
			cfg.metadataRedis[addr] = struct{}{}
		}
	}
	return cfg
}

// shardHasVoice reports whether any attendance guild lives on the shard.
func (cfg voiceConfig) shardHasVoice(shardID, shardCount int) bool {
	if shardCount <= 0 {
		return false
	}
	for guild := range cfg.guilds {
		if int((guild>>22)%int64(shardCount)) == shardID {
			return true
		}
	}
	return false
}

// voiceState is the Session's voice bookkeeping. lastGood is the receipt time
// (unix nanos) of the last dispatch fully handled on this shard; it outlives a
// connection so an identify can say how far the previous one is vouched for.
type voiceState struct {
	cfg     voiceConfig
	enabled bool

	connID   atomic.Pointer[string]
	lastGood atomic.Int64

	mu      sync.Mutex
	pending []pendingVoiceLoss
}

type pendingVoiceLoss struct {
	reason       string
	knownThrough time.Time
}

func (s *Session) connectionID() string {
	if id := s.voice.connID.Load(); id != nil {
		return *id
	}
	return ""
}

// stampReceipt records receipt immediately after decode, before state handling
// and publication, so later processing delay never moves an event's time.
func (s *Session) stampReceipt(ev *discord.Event, now time.Time) {
	ev.Receipt = discord.Receipt{
		ShardID:      s.shardID,
		ConnectionID: s.connectionID(),
		ReceivedAt:   now.UTC(),
	}
}

// noteVoiceHandled moves the shard's vouched-for point forward once a
// dispatch has been stored and published without loss.
func (s *Session) noteVoiceHandled(ev *discord.Event) {
	if s.voice.enabled && !ev.Receipt.ReceivedAt.IsZero() {
		s.voice.lastGood.Store(ev.Receipt.ReceivedAt.UnixNano())
	}
}

func (s *Session) lastVoiceGood() (time.Time, bool) {
	n := s.voice.lastGood.Load()
	if n == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, n).UTC(), true
}

// recordVoiceLoss stores a loss for this shard and notifies consumers. When
// State cannot take the write, the loss is kept and retried by the heartbeat,
// which reports the shard not ready until it lands. While earlier losses are
// still queued (State is evidently failing) a new one is only queued, merged
// into a same-reason entry keeping the earlier cutoff, so the read loop never
// stalls on a dead database and the queue stays bounded.
func (s *Session) recordVoiceLoss(reason string, knownThrough time.Time) {
	if !s.voice.enabled {
		return
	}
	s.voice.mu.Lock()
	backlog := len(s.voice.pending) > 0
	merged := false
	for i := range s.voice.pending {
		if s.voice.pending[i].reason == reason {
			if knownThrough.Before(s.voice.pending[i].knownThrough) {
				s.voice.pending[i].knownThrough = knownThrough
			}
			merged = true
			break
		}
	}
	if !merged {
		s.voice.pending = append(s.voice.pending, pendingVoiceLoss{reason: reason, knownThrough: knownThrough})
	}
	s.voice.mu.Unlock()
	if !backlog {
		s.flushVoiceLosses()
	}
}

// flushVoiceLosses writes queued losses in order and reports whether none
// remain. A failed write stops the flush so order is preserved.
func (s *Session) flushVoiceLosses() bool {
	s.voice.mu.Lock()
	defer s.voice.mu.Unlock()
	for len(s.voice.pending) > 0 {
		next := s.voice.pending[0]
		ctx, cancel := context.WithTimeout(context.Background(), voiceDBTimeout)
		loss, err := s.stateDB.RecordVoiceLoss(ctx, s.shardID, next.reason, next.knownThrough)
		cancel()
		if err != nil {
			s.log.Error(context.Background(), "record voice loss", slog.Error(err), slog.F("reason", next.reason))
			return false
		}
		s.voice.pending = s.voice.pending[1:]
		s.log.Warn(context.Background(), "voice observation loss recorded",
			slog.F("reason", loss.Reason), slog.F("revision", loss.Revision),
			slog.F("known_through", loss.KnownThrough))
		s.notifyVoiceLoss(loss)
	}
	return true
}

type voiceLossNotice struct {
	ShardID         int    `json:"shardId"`
	Revision        string `json:"revision"`
	Reason          string `json:"reason"`
	KnownThroughUtc string `json:"knownThroughUtc"`
	DetectedAtUtc   string `json:"detectedAtUtc"`
}

// notifyVoiceLoss is best effort: consumers also read the loss from State, so a
// failed notification is logged and nothing more.
func (s *Session) notifyVoiceLoss(loss *state.VoiceLoss) {
	body, err := json.Marshal(voiceLossNotice{
		ShardID:         loss.ShardID,
		Revision:        strconv.FormatInt(loss.Revision, 10),
		Reason:          loss.Reason,
		KnownThroughUtc: loss.KnownThrough.UTC().Format(receiptTimeLayout),
		DetectedAtUtc:   loss.DetectedAt.UTC().Format(receiptTimeLayout),
	})
	if err != nil {
		return
	}
	for _, rc := range s.rc {
		addr := rc.Options().Addr
		whitelist, ok := s.whitelistedEvents[addr]
		if !ok {
			continue
		}
		if _, ok := whitelist[VoiceLossEvent]; !ok {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), voiceDBTimeout)
		if err := rc.RPush(ctx, "gateway:events:"+VoiceLossEvent, body).Err(); err != nil {
			s.log.Error(ctx, "push voice loss notice", slog.Error(err), slog.F("redis_addr", addr))
		}
		cancel()
	}
}

// onVoiceIdentify runs on READY, which follows only an IDENTIFY: whatever
// happened between the previous connection and this one was not replayed.
func (s *Session) onVoiceIdentify(sessionID string) {
	s.voice.connID.Store(&sessionID)
	if !s.voice.enabled {
		return
	}
	knownThrough, ok := s.lastVoiceGood()
	if !ok {
		// A fresh process vouches for nothing; the previous process's last
		// heartbeat is the latest point anyone confirmed.
		ctx, cancel := context.WithTimeout(context.Background(), voiceDBTimeout)
		at, found, err := s.stateDB.GetVoiceShardHeartbeat(ctx, s.shardID)
		cancel()
		switch {
		case err == nil && found:
			knownThrough = at
		default:
			knownThrough = time.Now().UTC()
		}
	}
	s.recordVoiceLoss(statepsql.VoiceLossIdentifyNoResume, knownThrough)
}

// onVoiceResume keeps the connection identity across a successful RESUME.
func (s *Session) onVoiceResume(sessionID string) {
	s.voice.connID.Store(&sessionID)
}

// voiceGuildOf extracts guild_id from a raw dispatch without a full decode.
func voiceGuildOf(raw []byte) int64 {
	id, err := strconv.ParseInt(jsoniter.Get(raw, "guild_id").ToString(), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// voiceMetadataPayload adds the Eventer-only _tatsu receipt object to a
// VOICE_STATE_UPDATE for an attendance guild. It returns nil when the event
// needs no metadata, and never alters the payload other destinations get.
func (s *Session) voiceMetadataPayload(ev *discord.Event) []byte {
	if !s.voice.enabled || ev.T != "VOICE_STATE_UPDATE" || len(s.voice.cfg.metadataRedis) == 0 {
		return nil
	}
	if _, ok := s.voice.cfg.guilds[voiceGuildOf(ev.D)]; !ok {
		return nil
	}
	return withReceiptMetadata(ev)
}

type receiptMetadata struct {
	V               int    `json:"v"`
	ShardID         int    `json:"shardId"`
	ConnectionID    string `json:"connectionId"`
	DiscordSequence string `json:"discordSequence"`
	ReceivedAtUtc   string `json:"receivedAtUtc"`
}

func withReceiptMetadata(ev *discord.Event) []byte {
	meta, err := json.Marshal(receiptMetadata{
		V:               1,
		ShardID:         ev.Receipt.ShardID,
		ConnectionID:    ev.Receipt.ConnectionID,
		DiscordSequence: strconv.FormatInt(ev.S, 10),
		ReceivedAtUtc:   ev.Receipt.ReceivedAt.UTC().Format(receiptTimeLayout),
	})
	if err != nil {
		return nil
	}
	body := bytes.TrimRight(ev.D, " \t\r\n")
	if len(body) < 2 || body[len(body)-1] != '}' {
		return nil
	}
	out := make([]byte, 0, len(body)+len(meta)+12)
	out = append(out, body[:len(body)-1]...)
	if len(bytes.TrimSpace(body[1:len(body)-1])) > 0 {
		out = append(out, ',')
	}
	out = append(out, `"_tatsu":`...)
	out = append(out, meta...)
	out = append(out, '}')
	return out
}

// voiceHeartbeat keeps this shard's readiness fresh while the connection is
// authenticated. It reports not ready while a loss is still unrecorded, and
// treats its own row vanishing mid-connection as a State cache reset.
func (c *conn) voiceHeartbeat() {
	s := c.s
	t := time.NewTicker(voiceHeartbeatInterval)
	defer t.Stop()

	var (
		wrote     bool
		lastWrite time.Time
	)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-t.C:
		}
		if !c.authed.Load() {
			continue
		}
		connID := s.connectionID()
		if connID == "" {
			continue
		}

		ready := s.flushVoiceLosses()
		ctx, cancel := context.WithTimeout(c.ctx, voiceDBTimeout)
		created, err := s.stateDB.HeartbeatVoiceShard(ctx, s.shardID, connID, ready)
		cancel()
		if err != nil {
			s.log.Error(c.ctx, "voice readiness heartbeat", slog.Error(err))
			continue
		}
		if created && wrote {
			// The row this connection wrote is gone: the UNLOGGED cache was
			// emptied underneath us, guild baselines included.
			s.recordVoiceLoss(statepsql.VoiceLossStateCacheReset, lastWrite)
			if s.voice.cfg.reidentify {
				s.log.Warn(c.ctx, "voice cache reset; re-identifying for fresh baselines")
				s.ForceIdentify()
				return
			}
		}
		wrote = true
		lastWrite = time.Now().UTC()
	}
}
