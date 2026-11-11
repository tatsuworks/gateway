package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/discord/discordjson"
	"github.com/tatsuworks/gateway/internal/state"
)

type snapshotDB struct {
	state.DB
	snap  *state.VoiceSnapshot
	err   error
	after int64
	age   time.Duration
}

func (f *snapshotDB) Encoding() discord.Encoding { return discordjson.Encoding }
func (f *snapshotDB) GetGuildVoiceSnapshot(_ context.Context, _ int64, after int64, age time.Duration) (*state.VoiceSnapshot, error) {
	f.after, f.age = after, age
	return f.snap, f.err
}

func serveVoiceState(t *testing.T, db *snapshotDB, url string) (*httptest.ResponseRecorder, voiceStateJSON) {
	t.Helper()
	s, _ := NewServer(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), db, "test")
	s.Init()
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body voiceStateJSON
	if rec.Code != http.StatusBadRequest {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body: %v (%s)", err, rec.Body.String())
		}
	}
	return rec, body
}

func readySnapshot() *state.VoiceSnapshot {
	isBot := false
	at := time.Date(2026, 11, 10, 18, 0, 0, 500, time.UTC)
	return &state.VoiceSnapshot{
		GuildID: 173184118492889089, ShardID: 3, ConnectionID: "sess", ObservedAt: at,
		Ready: true, Complete: true,
		Occupants: []state.VoiceOccupant{{UserID: 1347103391402164315, ChannelID: 999999999999999999, IsBot: &isBot, ChangedAt: at, DiscordSeq: 12}},
		Loss:      &state.VoiceLoss{Revision: 9, ShardID: 3, Reason: "publish_failed", KnownThrough: at, DetectedAt: at},
	}
}

func TestGuildVoiceStateReady(t *testing.T) {
	db := &snapshotDB{snap: readySnapshot()}
	rec, body := serveVoiceState(t, db, "/v1/events/guilds/173184118492889089/voice-state?afterLossRevision=4")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if db.after != 4 || db.age != voiceReadinessMaxAge {
		t.Fatalf("query: after=%d age=%v", db.after, db.age)
	}
	o := body.Occupants[0]
	if body.GuildID != "173184118492889089" || *body.ShardID != 3 || o.DiscordUserID != "1347103391402164315" ||
		o.ChannelID != "999999999999999999" || o.DiscordSequence != "12" || *o.IsBot ||
		o.LastChangedAtUtc != "2026-11-10T18:00:00.000000Z" || body.Loss.Revision != "9" {
		t.Fatalf("ids and sequences must be exact decimal strings: %+v", body)
	}
}

func TestGuildVoiceStateNeverFabricatesPresence(t *testing.T) {
	notReady := readySnapshot()
	notReady.Ready, notReady.Complete, notReady.NotReadyReason = false, false, "stale_heartbeat"
	rec, body := serveVoiceState(t, &snapshotDB{snap: notReady}, "/v1/events/guilds/173184118492889089/voice-state")
	if rec.Code != http.StatusServiceUnavailable || body.Ready || len(body.Occupants) != 0 || body.Reason != "stale_heartbeat" || body.Loss == nil {
		t.Fatalf("unready must be 503 without occupants but with the loss: %d %+v", rec.Code, body)
	}

	incomplete := readySnapshot()
	incomplete.Complete = false
	rec, body = serveVoiceState(t, &snapshotDB{snap: incomplete}, "/v1/events/guilds/173184118492889089/voice-state")
	if rec.Code != http.StatusServiceUnavailable || body.Reason != "unclassified_occupant" || len(body.Occupants) != 0 {
		t.Fatalf("unclassified occupants make the source incomplete: %d %+v", rec.Code, body)
	}

	rec, body = serveVoiceState(t, &snapshotDB{err: errors.New("db down")}, "/v1/events/guilds/173184118492889089/voice-state")
	if rec.Code != http.StatusServiceUnavailable || body.Reason != "state_unavailable" || body.Ready {
		t.Fatalf("storage failure must be unavailable: %d %+v", rec.Code, body)
	}

	rec, _ = serveVoiceState(t, &snapshotDB{snap: readySnapshot()}, "/v1/events/guilds/173184118492889089/voice-state?afterLossRevision=-1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad revision must be 400, got %d", rec.Code)
	}
}
