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
	cfg := parseVoiceConfig(" 173184118492889089, nope ,42,", "valkey-karavan:6379", "")
	if _, ok := cfg.guilds[lounge]; !ok || len(cfg.guilds) != 1 {
		t.Fatalf("configuration must narrow, never widen, the approved guilds: %v", cfg.guilds)
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
	sources    []pendingVoiceLoss
}

func (f *lossFakeDB) RecordVoiceLoss(_ context.Context, shard int, reason string, knownThrough time.Time, connectionID string, sequence int64) (*state.VoiceLoss, error) {
	f.attempts++
	if f.down {
		return nil, errors.New("state down")
	}
	loss := state.VoiceLoss{Revision: int64(len(f.stored) + 1), ShardID: shard, Reason: reason, KnownThrough: knownThrough, DetectedAt: time.Now()}
	f.stored = append(f.stored, loss)
	f.sources = append(f.sources, pendingVoiceLoss{connectionID: connectionID, sequence: sequence})
	return &loss, nil
}

func (f *lossFakeDB) GetVoiceShardHeartbeat(context.Context, int) (time.Time, bool, error) {
	return f.heartbeat, f.hasHeartbe, nil
}

func TestVoiceLossesQueueWhileStateIsDownKeepingEarliestCutoff(t *testing.T) {
	db := &lossFakeDB{down: true}
	s := voiceSession(t, db)
	t0 := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)

	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, t0.Add(2*time.Second), "", 0)
	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, t0.Add(time.Second), "", 0)
	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, t0.Add(3*time.Second), "", 0)
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, t0.Add(4*time.Second), "", 0)
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

func TestQueuedVoiceLossKeepsCurrentSourceAndEarliestCutoff(t *testing.T) {
	db := &lossFakeDB{down: true}
	s := voiceSession(t, db)
	now := time.Now()
	s.onVoiceResume("old")
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, now, "old", 900)
	s.onVoiceResume("new")
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, now.Add(time.Second), "new", 2)
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, now.Add(2*time.Second), "new", 4)
	// A heartbeat finishing late on the old connection cannot hide current
	// corruption, nor can an older position on this connection.
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, now.Add(-time.Second), "old", 1000)
	s.recordVoiceLoss(statepsql.VoiceLossStateWriteFailed, now, "new", 3)
	db.down = false
	if !s.flushVoiceLosses() || len(db.stored) != 1 || db.sources[0].connectionID != "new" || db.sources[0].sequence != 4 || !db.stored[0].KnownThrough.Equal(now.Add(-time.Second)) {
		t.Fatalf("source=%+v losses=%+v", db.sources, db.stored)
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

	s.recordVoiceLoss(statepsql.VoiceLossPublishFailed, time.Date(2026, 11, 10, 18, 20, 0, 0, time.UTC), "", 0)
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

// sessionFakeDB returns a stored Discord session, as after a process restart.
type sessionFakeDB struct {
	lossFakeDB
	sessID string
}

func (f *sessionFakeDB) GetSessionID(context.Context, int, string) (string, error) {
	return f.sessID, nil
}

func TestRestartedProcessStampsReplayWithStoredSession(t *testing.T) {
	db := &sessionFakeDB{sessID: "sess-1"}
	s := voiceSession(t, db)
	s.loadSessID()
	stamp := func() string {
		ev := &discord.Event{}
		s.stampReceipt(ev, time.Now())
		return ev.Receipt.ConnectionID
	}
	if got := stamp(); got != "sess-1" {
		t.Fatalf("dispatches replayed before RESUMED must carry the stored session: %q", got)
	}
	s.onVoiceResume("sess-1")
	if got := stamp(); got != "sess-1" || len(db.stored) != 0 {
		t.Fatalf("a successful resume keeps the identity and is no loss: %q %+v", got, db.stored)
	}

	// INVALID_SESSION, then a fresh READY: a new identity and a loss.
	s.onVoiceIdentify("sess-2")
	if got := stamp(); got != "sess-2" || len(db.stored) != 1 || db.stored[0].Reason != statepsql.VoiceLossIdentifyNoResume {
		t.Fatalf("a fresh identify replaces the identity and records a loss: %q %+v", got, db.stored)
	}
}

// heartbeatFakeDB keeps one shard status row with State's pending rule.
// lostResults makes that many heartbeats apply but report an error, like a
// write that committed but whose answer never arrived.
type heartbeatFakeDB struct {
	lossFakeDB
	exists, pending bool
	connID          string
	lostResults     int
}

func (f *heartbeatFakeDB) HeartbeatVoiceShard(_ context.Context, _ int, connID string, _, resumed, wroteBefore bool) (bool, error) {
	if !f.exists {
		f.exists, f.pending = true, resumed || wroteBefore
	} else if f.connID != connID && !resumed {
		f.pending = false
	}
	f.connID = connID
	if f.lostResults > 0 {
		f.lostResults--
		return false, errors.New("timeout")
	}
	return f.pending, nil
}

func heartbeatConn(t *testing.T, db *heartbeatFakeDB, resumed bool) *conn {
	s := voiceSession(t, db)
	s.voice.cfg.reidentify = true
	id := "sess-1"
	s.voice.connID.Store(&id)
	c := &conn{s: s, ctx: context.Background(), cancel: func() {}}
	c.authed.Store(true)
	c.resumed.Store(resumed)
	return c
}

func TestHeartbeatTreatsRecreatedRowAsCacheReset(t *testing.T) {
	last := time.Date(2026, 11, 10, 18, 20, 0, 0, time.UTC)
	cases := []struct {
		name    string
		resumed bool
		ticks   int
		vanish  bool // the row disappears after the first tick
		reset   bool
	}{
		{"identified connection creates its first row", false, 2, false, false},
		{"row vanishes under a connection", false, 2, true, true},
		{"resumed connection finds no row (cache emptied while disconnected, or first deploy)", true, 1, false, true},
		{"resumed connection finds its row", true, 2, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &heartbeatFakeDB{}
			if tc.name == "resumed connection finds its row" {
				db.exists, db.connID = true, "sess-1"
			}
			c := heartbeatConn(t, db, tc.resumed)
			c.s.noteVoiceHandled(&discord.Event{Receipt: discord.Receipt{ReceivedAt: last}})
			var hb voiceHeartbeatState
			replaced := false
			for i := 0; i < tc.ticks; i++ {
				if c.voiceHeartbeatTick(&hb) {
					replaced = true
					break
				}
				if tc.vanish && i == 0 {
					db.exists = false
				}
			}
			if replaced != tc.reset || (len(db.stored) == 1) != tc.reset {
				t.Fatalf("reset=%v replaced=%v losses=%+v", tc.reset, replaced, db.stored)
			}
			if tc.reset && db.stored[0].Reason != statepsql.VoiceLossStateCacheReset {
				t.Fatalf("loss reason = %q", db.stored[0].Reason)
			}
			if tc.reset && tc.resumed && !db.stored[0].KnownThrough.Equal(last) {
				t.Fatalf("a resumed connection vouches through the last handled dispatch: %v", db.stored[0].KnownThrough)
			}
		})
	}
}

func TestHeartbeatRecoveryOutlivesALostAnswerAndARestart(t *testing.T) {
	// The resumed connection's insert commits but its answer is lost.
	db := &heartbeatFakeDB{lostResults: 1}
	c := heartbeatConn(t, db, true)
	var hb voiceHeartbeatState
	if c.voiceHeartbeatTick(&hb) || len(db.stored) != 0 {
		t.Fatalf("nothing is known from a lost answer")
	}

	// The process restarts and resumes the same session before the next tick.
	c = heartbeatConn(t, db, true)
	hb = voiceHeartbeatState{}
	if !c.voiceHeartbeatTick(&hb) || len(db.stored) != 1 || db.stored[0].Reason != statepsql.VoiceLossStateCacheReset {
		t.Fatalf("the pending baselines must still be recovered: losses=%+v", db.stored)
	}

	// Until a fresh identify, every connection still sees them pending.
	if !db.pending {
		t.Fatalf("recovery is pending until a freshly identified connection heartbeats")
	}
	fresh := heartbeatConn(t, db, false)
	id := "sess-2"
	fresh.s.voice.connID.Store(&id)
	hb = voiceHeartbeatState{}
	if fresh.voiceHeartbeatTick(&hb) || db.pending {
		t.Fatalf("a freshly identified connection clears the mark: pending=%v", db.pending)
	}
}

func TestVoiceRecoveryRollingWindow(t *testing.T) {
	s := voiceSession(t, &lossFakeDB{})
	now := time.Now()
	if !s.requestVoiceIdentify(now) {
		t.Fatal("first qualifying failure must recover")
	}
	s.onVoiceIdentify("fresh-session")
	s.onVoiceResume("fresh-session")
	for _, offset := range []time.Duration{time.Second, 9 * time.Minute, 10*time.Minute - time.Nanosecond} {
		if s.requestVoiceIdentify(now.Add(offset)) {
			t.Fatalf("READY/RESUMED must not reset the rolling window: %v", offset)
		}
	}
	if !s.requestVoiceIdentify(now.Add(10 * time.Minute)) {
		t.Fatal("a fresh failure at expiry may recover again")
	}
	if s.requestVoiceIdentify(now.Add(11 * time.Minute)) {
		t.Fatal("the new attempt starts another ten-minute window")
	}
	s.voice.cfg.reidentify = false
	if s.requestVoiceIdentify(now.Add(time.Hour)) {
		t.Fatal("the existing explicit disable must still work")
	}
}

func TestVoiceRecoveryBudgetSurvivesReconnects(t *testing.T) {
	db := &heartbeatFakeDB{}
	c := heartbeatConn(t, db, true)
	var hb voiceHeartbeatState
	if !c.voiceHeartbeatTick(&hb) {
		t.Fatal("first-rollout resumed connection must recover its missing baseline")
	}

	// A new connection, not a new Session: its own heartbeat history resets,
	// but repeated cache failures must not keep re-identifying the shard.
	c = &conn{s: c.s, ctx: context.Background(), cancel: func() {}}
	c.authed.Store(true)
	c.resumed.Store(true)
	hb = voiceHeartbeatState{}
	if c.voiceHeartbeatTick(&hb) {
		t.Fatal("a reconnect must not replenish the shard's voice recovery budget")
	}
	if !db.pending {
		t.Fatal("suppression must leave baseline recovery pending, not claim readiness")
	}
}

func TestHeartbeatRecordsAPendingResetOncePerConnection(t *testing.T) {
	db := &heartbeatFakeDB{}
	c := heartbeatConn(t, db, true)
	c.s.voice.cfg.reidentify = false
	var hb voiceHeartbeatState
	for i := 0; i < 3; i++ {
		if c.voiceHeartbeatTick(&hb) {
			t.Fatalf("no identify when re-identify is off")
		}
	}
	if len(db.stored) != 1 || !db.pending {
		t.Fatalf("one loss, and the baselines stay pending: losses=%d pending=%v", len(db.stored), db.pending)
	}
}
