package gatewayws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/redis/go-redis/v9"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/internal/state"
	"github.com/tatsuworks/gateway/internal/state/db/statepsql"
)

const (
	lounge       = int64(173184118492889089)
	loungeShards = 1024
)

func loungeShard() int { return int((lounge >> 22) % loungeShards) }

func TestParseVoiceConfigScopesShardsAndDestinations(t *testing.T) {
	cfg := parseVoiceConfig(" 173184118492889089, nope ,", "valkey-karavan:6379", "")
	if _, ok := cfg.guilds[lounge]; !ok || len(cfg.guilds) != 1 {
		t.Fatalf("guilds: %v", cfg.guilds)
	}
	if _, ok := cfg.metadataRedis["valkey-karavan:6379"]; !ok || !cfg.reidentify {
		t.Fatalf("destinations/reidentify: %+v", cfg)
	}
	if !cfg.shardHasVoice(loungeShard(), loungeShards) || cfg.shardHasVoice(loungeShard()+1, loungeShards) {
		t.Fatalf("only the shard hosting an attendance guild does voice work")
	}
	if off := parseVoiceConfig("", "", "false"); len(off.guilds) != 0 || off.reidentify || off.shardHasVoice(0, 1) {
		t.Fatalf("empty configuration must disable voice work: %+v", off)
	}
}

func voiceEvent(guild string) *discord.Event {
	return &discord.Event{
		T: "VOICE_STATE_UPDATE",
		S: 123456789012,
		D: []byte(`{"guild_id":"` + guild + `","user_id":"7","channel_id":"5"}`),
		Receipt: discord.Receipt{
			ShardID:      loungeShard(),
			ConnectionID: "sess-1",
			ReceivedAt:   time.Date(2026, 11, 10, 18, 15, 0, 750000000, time.UTC),
		},
	}
}

func TestReceiptMetadataIsAdditive(t *testing.T) {
	out := withReceiptMetadata(voiceEvent("173184118492889089"))
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("payload must stay valid JSON: %v (%s)", err, out)
	}
	if string(decoded["guild_id"]) != `"173184118492889089"` || string(decoded["channel_id"]) != `"5"` {
		t.Fatalf("original fields must be preserved: %s", out)
	}
	var meta receiptMetadata
	if err := json.Unmarshal(decoded["_tatsu"], &meta); err != nil {
		t.Fatalf("_tatsu: %v", err)
	}
	want := receiptMetadata{V: 1, ShardID: loungeShard(), ConnectionID: "sess-1", DiscordSequence: "123456789012", ReceivedAtUtc: "2026-11-10T18:15:00.750000Z"}
	if meta != want {
		t.Fatalf("metadata = %+v, want %+v", meta, want)
	}
	if _, ok := decoded["timestamp"]; ok {
		t.Fatalf("metadata must not add the top-level timestamp Eventer's 30s filter reads")
	}

	empty := withReceiptMetadata(&discord.Event{D: []byte(`{}`), Receipt: voiceEvent("1").Receipt})
	if err := json.Unmarshal(empty, &decoded); err != nil {
		t.Fatalf("empty object: %v (%s)", err, empty)
	}
}

func voiceSession(t *testing.T, db state.DB) *Session {
	s := &Session{log: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), shardID: loungeShard(), shardCount: loungeShards, stateDB: db}
	s.voice.cfg = parseVoiceConfig("173184118492889089", "", "")
	s.voice.enabled = true
	return s
}

func TestVoiceMetadataOnlyForAttendanceGuildsAndDestinations(t *testing.T) {
	s := voiceSession(t, nil)
	s.voice.cfg.metadataRedis = map[string]struct{}{"eventer:6379": {}}
	if s.voiceMetadataPayload(voiceEvent("42")) != nil {
		t.Fatalf("other guilds' voice events stay unchanged")
	}
	if s.voiceMetadataPayload(voiceEvent("173184118492889089")) == nil {
		t.Fatalf("attendance guild voice events gain metadata")
	}
	msg := voiceEvent("173184118492889089")
	msg.T = "MESSAGE_CREATE"
	if s.voiceMetadataPayload(msg) != nil {
		t.Fatalf("only VOICE_STATE_UPDATE gains metadata")
	}
}

// localRedis returns a client on a dedicated logical database of the local
// Redis, or skips when none is running.
func localRedis(t *testing.T, addr string, db int) *redis.Client {
	t.Helper()
	rc := redis.NewClient(&redis.Options{Addr: addr, DB: db, MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	if err := rc.Ping(context.Background()).Err(); err != nil {
		t.Skipf("skipping: redis not available at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = rc.FlushDB(context.Background()).Err(); _ = rc.Close() })
	_ = rc.FlushDB(context.Background()).Err()
	return rc
}

func TestPushEventToRedisKeepsWorkerPayloadAndReportsLoss(t *testing.T) {
	eventer := localRedis(t, "localhost:6379", 14)
	worker := localRedis(t, "127.0.0.1:6379", 15)
	s := voiceSession(t, nil)
	s.voice.cfg.metadataRedis = map[string]struct{}{"localhost:6379": {}}
	s.rc = []*redis.Client{eventer, worker}
	c := &conn{s: s, ctx: context.Background()}

	ev := voiceEvent("173184118492889089")
	if !c.pushEventToRedis(ev) {
		t.Fatalf("successful publication must not report a loss")
	}
	got, err := worker.LPop(context.Background(), "gateway:events:VOICE_STATE_UPDATE").Bytes()
	if err != nil || string(got) != string(ev.D) {
		t.Fatalf("worker payload must be byte-identical: %s (%v)", got, err)
	}
	got, err = eventer.LPop(context.Background(), "gateway:events:VOICE_STATE_UPDATE").Bytes()
	if err != nil || string(got) != string(withReceiptMetadata(ev)) {
		t.Fatalf("eventer payload must carry metadata: %s (%v)", got, err)
	}

	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	defer dead.Close()
	s.rc = []*redis.Client{dead, worker}
	s.voice.cfg.metadataRedis = map[string]struct{}{"127.0.0.1:1": {}}
	if c.pushEventToRedis(ev) {
		t.Fatalf("a failed push to the attendance destination is a loss")
	}
	s.voice.cfg.metadataRedis = map[string]struct{}{"localhost:6379": {}}
	if !c.pushEventToRedis(ev) {
		t.Fatalf("failures elsewhere stay logged only, as before")
	}
}

// lossFakeDB fails RecordVoiceLoss while down and records what it stored.
type lossFakeDB struct {
	state.DB
	down       bool
	stored     []state.VoiceLoss
	attempts   int
	heartbeat  time.Time
	hasHeartbe bool
}

func (f *lossFakeDB) RecordVoiceLoss(_ context.Context, shard int, reason string, knownThrough time.Time) (*state.VoiceLoss, error) {
	f.attempts++
	if f.down {
		return nil, errors.New("state down")
	}
	loss := state.VoiceLoss{Revision: int64(len(f.stored) + 1), ShardID: shard, Reason: reason, KnownThrough: knownThrough, DetectedAt: time.Now()}
	f.stored = append(f.stored, loss)
	return &loss, nil
}

func (f *lossFakeDB) GetVoiceShardHeartbeat(context.Context, int) (time.Time, bool, error) {
	return f.heartbeat, f.hasHeartbe, nil
}

func TestVoiceLossesQueueWhileStateIsDownKeepingEarliestCutoff(t *testing.T) {
	db := &lossFakeDB{down: true}
	s := voiceSession(t, db)
	t0 := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)

	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, t0.Add(2*time.Second))
	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, t0.Add(time.Second))
	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, t0.Add(3*time.Second))
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, t0.Add(4*time.Second))
	if db.attempts != 1 {
		t.Fatalf("with a backlog the read loop must not retry State per event: %d attempts", db.attempts)
	}
	if s.flushVoiceLosses() {
		t.Fatalf("flush must report losses still pending while State is down")
	}

	db.down = false
	if !s.flushVoiceLosses() {
		t.Fatalf("flush must drain once State recovers")
	}
	if len(db.stored) != 2 || !db.stored[0].KnownThrough.Equal(t0.Add(time.Second)) || db.stored[1].Reason != statepsql.VoiceLossStateWriteFailed {
		t.Fatalf("queued losses must keep order and the earliest cutoff: %+v", db.stored)
	}
}

func TestVoiceLossNoticeGoesOnlyToDestinationsThatAskForIt(t *testing.T) {
	eventer := localRedis(t, "localhost:6379", 14)
	worker := localRedis(t, "127.0.0.1:6379", 15)
	db := &lossFakeDB{}
	s := voiceSession(t, db)
	s.rc = []*redis.Client{eventer, worker}
	s.whitelistedEvents = map[string]map[string]struct{}{
		"localhost:6379": {VoiceLossEvent: {}},
		"127.0.0.1:6379": {"VOICE_STATE_UPDATE": {}},
	}

	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, time.Date(2026, 11, 10, 18, 20, 0, 0, time.UTC))
	raw, err := eventer.LPop(context.Background(), "gateway:events:"+VoiceLossEvent).Bytes()
	if err != nil {
		t.Fatalf("eventer must get the notice: %v", err)
	}
	var notice voiceLossNotice
	if err := json.Unmarshal(raw, &notice); err != nil || notice.Revision != "1" || notice.KnownThroughUtc != "2026-11-10T18:20:00.000000Z" || notice.ShardID != loungeShard() {
		t.Fatalf("notice = %+v (%v)", notice, err)
	}
	if n, _ := worker.Exists(context.Background(), "gateway:events:"+VoiceLossEvent).Result(); n != 0 {
		t.Fatalf("a destination that did not whitelist the notice must not get it")
	}
}

func TestIdentifyRecordsLossFromLastVouchedPoint(t *testing.T) {
	last := time.Date(2026, 11, 10, 18, 20, 0, 0, time.UTC)

	db := &lossFakeDB{}
	s := voiceSession(t, db)
	s.noteVoiceHandled(&discord.Event{Receipt: discord.Receipt{ReceivedAt: last}})
	s.onVoiceIdentify("sess-2")
	if s.connectionID() != "sess-2" || len(db.stored) != 1 || db.stored[0].Reason != statepsql.VoiceLossIdentifyNoResume || !db.stored[0].KnownThrough.Equal(last) {
		t.Fatalf("identify must change identity and record a loss through the last handled receipt: %+v", db.stored)
	}

	fresh := &lossFakeDB{heartbeat: last.Add(-time.Minute), hasHeartbe: true}
	s = voiceSession(t, fresh)
	s.onVoiceIdentify("sess-3")
	if len(fresh.stored) != 1 || !fresh.stored[0].KnownThrough.Equal(last.Add(-time.Minute)) {
		t.Fatalf("a fresh process vouches only through the previous heartbeat: %+v", fresh.stored)
	}

	s.onVoiceResume("sess-3")
	if len(fresh.stored) != 1 {
		t.Fatalf("a successful resume is not a loss")
	}
}
