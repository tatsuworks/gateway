# Faster Mass Reconnect — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the fixed `IDENTIFY_STABILIZE_SECONDS` post-READY wait with a per-shard chunk-drain signal, bounded by the existing max-duration safety cap.

**Architecture:** After READY, a shard that may request guild members acquires the existing stabilize semaphore. Phase 1 holds it for a fixed duration (default 60s). Phase 2 replaces that with: wait until the shard's outstanding `GUILD_MEMBERS_CHUNK` drain count reaches zero, OR until the max-duration safety cap fires. The tracker is per-`Session`, in-process, and fed by hooking `requestGuildMembers` (registers expected drain) and the event-loop `GUILD_MEMBERS_CHUNK` branch (records arrivals). Decoders are extended to read `chunk_index`/`chunk_count` so the tracker can recognise completion.

**Tech Stack:** Go 1.21+, `nhooyr.io/websocket`, ETF + JSON decoders in `discord/discordetf` and `discord/discordjson`, `internal/gatewayws.Session`.

**Reference spec:** `docs/superpowers/specs/2026-04-30-faster-mass-reconnect-design.md` (Phase 2 section).

---

## Background / Touch Points

Phase 1 already shipped (commits `fb682c2`, `f2474df`, `ff61f07`):

- `Session.holdStabilize()` (`internal/gatewayws/ws.go:629-669`) acquires `stabilizeSem`, sleeps for `stabilizeDuration`, releases. Bound by `stabilizeMaxDuration` (default 2× stabilizeDuration).
- `requestGuildMembers(guild int64)` lives in `internal/gatewayws/write.go:187` — sends Discord op 8.
- Member chunks decoded by `discordetf.DecodeMemberChunk` (`discord/discordetf/member.go`) and `discordjson.DecodeMemberChunk` (`discord/discordjson/member.go`). Neither currently reads `chunk_index` or `chunk_count`.
- Chunk routing: ws.go event loop → `s.state.HandleEvent(ctx, ev)` (handler/helpers.go:44, key `"GUILD_MEMBERS_CHUNK"`) → `handler.Client.MemberChunk` (`handler/member.go:10`).
- `handler.EventPayload` (`handler/helpers.go:11`) is the existing channel for handler→ws-layer metadata; already carries `GuildID` and `DiscordMemberCount`.

Pre-existing minor bug noticed but **out of scope**: `pushEventToRedis` skip check at `internal/gatewayws/ws.go:466` uses `"GUILD_MEMBER_CHUNK"` (singular) — Discord's actual event name is `"GUILD_MEMBERS_CHUNK"` (plural, matches handler/helpers.go:44). Do not fix as part of this plan; leave a follow-up note.

## File Map

| File | Action | Responsibility |
|---|---|---|
| `discord/types.go` | Modify | Extend `MemberChunk` struct with chunk-pagination fields |
| `discord/discordetf/member.go` | Modify | Decode `chunk_index` + `chunk_count` keys |
| `discord/discordjson/member.go` | Modify | Decode `chunk_index` + `chunk_count` keys |
| `discord/discordjson/member_test.go` | Create | Unit-test JSON decoder additions |
| `handler/helpers.go` | Modify | Surface chunk index/count through `EventPayload` |
| `handler/member.go` | Modify | `MemberChunk` returns the chunk metadata via `EventPayload` |
| `internal/gatewayws/chunk_tracker.go` | Create | Per-session drain tracker (registry, signal channel, counters) |
| `internal/gatewayws/chunk_tracker_test.go` | Create | Unit tests for tracker semantics |
| `internal/gatewayws/ws.go` | Modify | Wire tracker register/record + drain-wait in `holdStabilize` |
| `internal/gatewayws/write.go` | Modify | `requestGuildMembers` registers expected drain |
| `internal/manager/manager.go` | Modify | Read new env `IDENTIFY_STABILIZE_USE_DRAIN`, propagate via `SessionConfig` |

No DB or schema changes. No new dependencies.

## Knobs (env)

| Env | Default | Effect |
|---|---|---|
| `IDENTIFY_STABILIZE_USE_DRAIN` | `true` | When true, `holdStabilize` waits on drain instead of fixed sleep. When false, falls back to Phase 1 fixed sleep. Rollback knob. |
| `IDENTIFY_STABILIZE_SECONDS` | **`5s` when drain enabled / `60s` when drain disabled** | Floor when drain is enabled (min hold per shard); fixed hold when drain is disabled (Phase 1 semantics). Default chosen by `IDENTIFY_STABILIZE_USE_DRAIN`. |
| `IDENTIFY_STABILIZE_SECONDS_MAX` | `120s` | Hard cap on per-shard hold. Decoupled from the floor (previously coupled as `2 × floor`). Fires only when drain is stalled — speedup comes from the floor, not the cap. |
| `IDENTIFY_STABILIZE_CONCURRENCY` | `1` | Unchanged from Phase 1. |

The floor exists because a shard with zero outstanding chunk requests (divergence-on + every guild skipped, or `SKIP_MEMBER_REQUEST=true`) would otherwise release the semaphore instantly. Some pacing is still useful — but only a small amount. The merge-time default (5s when drain is on) is well below typical drain time, so for shards that *do* fetch members the floor never gates: the drain signal does.

The cap is decoupled from the floor — the previous "2 × floor" coupling was an artifact, not a design intent. Default 120s matches Phase 1's existing cap (which empirically covers the worst legitimate drain) and is irrelevant to the speedup: the cap only fires when drain is stalled. Speedup comes entirely from dropping the floor.

The rollback path (`IDENTIFY_STABILIZE_USE_DRAIN=false`) automatically reverts the floor default to 60s so Phase 1 semantics are reproduced exactly. Ops can also set both knobs explicitly to override.

## Zero-chunk shards

Three cases must release the semaphore promptly:

1. **`SKIP_MEMBER_REQUEST=true` or `!hasGuildMembersIntent`** — `holdStabilize` is already not invoked (ws.go:559 guard). No change.
2. **Divergence check skipped every guild** — `requestGuildMembers` is never called, tracker stays at zero. Drain wait completes immediately; the floor (`IDENTIFY_STABILIZE_SECONDS`, default 5s with drain on) bounds the release.
3. **Discord sends zero chunks for a requested guild** — should not happen in practice but possible for a guild deleted between request and response. The tracker uses `chunk_count` from the first arriving chunk as the expected total; if no chunk arrives, the per-guild entry sits until the global max-duration cap (`IDENTIFY_STABILIZE_SECONDS_MAX`, default 120s) fires.

## Out of scope (Phase 2)

- Batcher-queue-depth signal (spec mentions "AND batcher queue depth below threshold"). Deferred — chunk-drain alone is sufficient for first cut. Revisit if metrics show backfill bursts overrunning batcher capacity at raised concurrency.
- Tracker persistence across reconnects. Tracker is per-session; a reconnect resets it. This is correct: a fresh connection re-issues identifies and re-requests members where divergence demands it.
- Exposing tracker state via the gRPC management API.

---

## Tasks

### Task 1: Extend `MemberChunk` type with pagination fields

**Files:**
- Modify: `discord/types.go:49-52`

- [ ] **Step 1: Edit the struct**

```go
type MemberChunk struct {
    GuildID    int64
    Members    map[int64][]byte
    ChunkIndex int32
    ChunkCount int32
}
```

- [ ] **Step 2: Build to confirm callers still compile**

Run: `cd /home/benson/gateway && go build ./...`
Expected: success (decoders set zero-valued fields until next task).

- [ ] **Step 3: Commit**

```bash
git add discord/types.go
git commit -m "discord: add ChunkIndex/ChunkCount to MemberChunk"
```

---

### Task 2: Decode `chunk_index`/`chunk_count` in JSON decoder + test

**Files:**
- Modify: `discord/discordjson/member.go:10-29`
- Create: `discord/discordjson/member_test.go`

- [ ] **Step 1: Write the failing test**

Create `discord/discordjson/member_test.go`:

```go
package discordjson

import (
	"testing"
)

func TestDecodeMemberChunk_PaginationFields(t *testing.T) {
	// minimal Discord GUILD_MEMBERS_CHUNK payload
	payload := []byte(`{
		"guild_id":"41771983444115456",
		"members":[{"user":{"id":"80351110224678912"}}],
		"chunk_index":3,
		"chunk_count":10
	}`)

	mc, err := (decoder{}).DecodeMemberChunk(payload)
	if err != nil {
		t.Fatalf("decode err: %v", err)
	}
	if mc.GuildID != 41771983444115456 {
		t.Fatalf("guild_id mismatch: %d", mc.GuildID)
	}
	if mc.ChunkIndex != 3 {
		t.Fatalf("chunk_index: got %d want 3", mc.ChunkIndex)
	}
	if mc.ChunkCount != 10 {
		t.Fatalf("chunk_count: got %d want 10", mc.ChunkCount)
	}
}

func TestDecodeMemberChunk_PaginationDefaults(t *testing.T) {
	payload := []byte(`{
		"guild_id":"41771983444115456",
		"members":[]
	}`)

	mc, err := (decoder{}).DecodeMemberChunk(payload)
	if err != nil {
		t.Fatalf("decode err: %v", err)
	}
	if mc.ChunkIndex != 0 || mc.ChunkCount != 0 {
		t.Fatalf("expected zero defaults, got %d/%d", mc.ChunkIndex, mc.ChunkCount)
	}
}
```

- [ ] **Step 2: Run test to confirm failure**

Run: `cd /home/benson/gateway && go test ./discord/discordjson/ -run TestDecodeMemberChunk_Pagination`
Expected: FAIL — `chunk_index: got 0 want 3`

- [ ] **Step 3: Implement decoder change**

Replace the body of `DecodeMemberChunk` in `discord/discordjson/member.go` with:

```go
func (_ decoder) DecodeMemberChunk(buf []byte) (*discord.MemberChunk, error) {
	var (
		mc  discord.MemberChunk
		err error
	)

	var members []jsoniter.RawMessage
	jsoniter.Get(buf, "members").ToVal(&members)
	mc.Members, err = nestedRawsToMapBySnowflake(members, "user")
	if err != nil {
		return nil, xerrors.Errorf("map members by id: %w", err)
	}

	mc.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	if v := jsoniter.Get(buf, "chunk_index"); v.LastError() == nil {
		mc.ChunkIndex = int32(v.ToInt())
	}
	if v := jsoniter.Get(buf, "chunk_count"); v.LastError() == nil {
		mc.ChunkCount = int32(v.ToInt())
	}

	return &mc, nil
}
```

- [ ] **Step 4: Run test to confirm pass**

Run: `cd /home/benson/gateway && go test ./discord/discordjson/`
Expected: PASS for both new tests; existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add discord/discordjson/member.go discord/discordjson/member_test.go
git commit -m "discordjson: decode MemberChunk pagination fields"
```

---

### Task 3: Decode `chunk_index`/`chunk_count` in ETF decoder

**Files:**
- Modify: `discord/discordetf/member.go:9-47`

No new test file — the ETF helpers (`etfDecoder`) are not directly exercised by existing tests, and the JSON test in Task 2 anchors the contract. We rely on the build + a manual review.

- [ ] **Step 1: Inspect available ETF reader helpers**

Run: `cd /home/benson/gateway && grep -n "readSmallInt\|readInt\|readSmallBig\|readSignedInt\|readInteger" discord/discordetf/*.go`
Expected: find an integer-decoding helper. Discord sends `chunk_index`/`chunk_count` as small ETF integers (typically tag `ettSmallInt` / `ettInt`).

- [ ] **Step 2: Add cases for the two keys**

Edit the `switch key` block in `discord/discordetf/member.go`:

```go
switch key {
case "guild_id":
    mc.GuildID, err = d.readSmallBigWithTagToInt64()
    if err != nil {
        return nil, xerrors.Errorf("extract guild_id from map: %w", err)
    }

case "members":
    mc.Members, err = d.readListIntoMapByID()
    if err != nil {
        return nil, xerrors.Errorf("extract members list from map: %w", err)
    }

case "chunk_index":
    n, err := d.readIntegerWithTag() // pick the helper found in Step 1
    if err != nil {
        return nil, xerrors.Errorf("extract chunk_index from map: %w", err)
    }
    mc.ChunkIndex = int32(n)

case "chunk_count":
    n, err := d.readIntegerWithTag()
    if err != nil {
        return nil, xerrors.Errorf("extract chunk_count from map: %w", err)
    }
    mc.ChunkCount = int32(n)

default:
}
```

**Note:** If `readIntegerWithTag` does not exist, locate the equivalent. Common patterns in this codebase: `readSmallIntWithTag` for `ettSmallInt`, `readInt32WithTag` for `ettInt`. The two ETF integer tags can both appear; if there is no single helper, peek the next byte and dispatch. Do **not** silently skip — that would mask a Discord protocol change.

- [ ] **Step 3: Build**

Run: `cd /home/benson/gateway && go build ./...`
Expected: success.

- [ ] **Step 4: Run all decoder tests**

Run: `cd /home/benson/gateway && go test ./discord/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add discord/discordetf/member.go
git commit -m "discordetf: decode MemberChunk pagination fields"
```

---

### Task 4: Surface chunk metadata through `EventPayload`

**Files:**
- Modify: `handler/helpers.go:11-18` + the `GUILD_MEMBERS_CHUNK` switch arm at `:44`
- Modify: `handler/member.go:10-22`

- [ ] **Step 1: Extend `EventPayload`**

In `handler/helpers.go`:

```go
type EventPayload struct {
	GuildID int64

	// DiscordMemberCount is the member_count field reported by Discord
	// in the GUILD_CREATE/GUILD_UPDATE payload, or 0 when not present.
	// Used by gatewayws to decide whether divergence checking applies.
	DiscordMemberCount int64

	// MemberChunkIndex / MemberChunkCount are only populated for
	// GUILD_MEMBERS_CHUNK events. Both zero means the event was not a
	// member chunk; ChunkCount > 0 with ChunkIndex == ChunkCount-1
	// indicates the final chunk for a guild's drain window.
	MemberChunkIndex int32
	MemberChunkCount int32
}
```

- [ ] **Step 2: Update `Client.MemberChunk` to return the payload**

In `handler/member.go`:

```go
func (c *Client) MemberChunk(ctx context.Context, d []byte) (*EventPayload, error) {
	mc, err := c.enc.DecodeMemberChunk(d)
	if err != nil {
		return nil, err
	}

	err = c.db.SetGuildMembers(ctx, mc.GuildID, mc.Members)
	if err != nil {
		c.log.Error(ctx, "failed to set members", slog.Error(err))
	}

	return &EventPayload{
		GuildID:          mc.GuildID,
		MemberChunkIndex: mc.ChunkIndex,
		MemberChunkCount: mc.ChunkCount,
	}, nil
}
```

- [ ] **Step 3: Update the `GUILD_MEMBERS_CHUNK` arm in `HandleEvent`**

In `handler/helpers.go`:

```go
case "GUILD_MEMBERS_CHUNK":
    return c.MemberChunk(ctx, e.D)
```

(Replaces the existing `return nil, c.MemberChunk(ctx, e.D)`.)

- [ ] **Step 4: Build**

Run: `cd /home/benson/gateway && go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add handler/helpers.go handler/member.go
git commit -m "handler: surface chunk index/count via EventPayload"
```

---

### Task 5: Add the drain tracker

**Files:**
- Create: `internal/gatewayws/chunk_tracker.go`
- Create: `internal/gatewayws/chunk_tracker_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/gatewayws/chunk_tracker_test.go`:

```go
package gatewayws

import (
	"testing"
	"time"
)

func TestChunkTracker_DrainCompletesAfterAllChunks(t *testing.T) {
	tr := newChunkTracker()
	tr.registerRequest(123)

	if tr.pendingCount() != 1 {
		t.Fatalf("pending: got %d want 1", tr.pendingCount())
	}

	tr.recordChunk(123, 0, 3)
	tr.recordChunk(123, 1, 3)
	if tr.pendingCount() != 1 {
		t.Fatalf("should still be pending after 2/3 chunks")
	}

	tr.recordChunk(123, 2, 3)
	select {
	case <-tr.drained():
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("drained channel did not fire after all chunks arrived")
	}
	if tr.pendingCount() != 0 {
		t.Fatalf("pending: got %d want 0", tr.pendingCount())
	}
}

func TestChunkTracker_DrainedFiresImmediatelyWhenEmpty(t *testing.T) {
	tr := newChunkTracker()
	select {
	case <-tr.drained():
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("empty tracker should report drained immediately")
	}
}

func TestChunkTracker_MultipleGuilds(t *testing.T) {
	tr := newChunkTracker()
	tr.registerRequest(1)
	tr.registerRequest(2)
	tr.recordChunk(1, 0, 1)
	if tr.pendingCount() != 1 {
		t.Fatalf("guild 1 drained, guild 2 still pending; got %d", tr.pendingCount())
	}
	tr.recordChunk(2, 0, 2)
	tr.recordChunk(2, 1, 2)
	if tr.pendingCount() != 0 {
		t.Fatalf("both guilds should be drained; got %d", tr.pendingCount())
	}
}

func TestChunkTracker_UnregisteredChunkIgnored(t *testing.T) {
	tr := newChunkTracker()
	tr.recordChunk(999, 0, 1) // never registered
	if tr.pendingCount() != 0 {
		t.Fatalf("unregistered guild should not create pending entry")
	}
}
```

- [ ] **Step 2: Run tests to confirm failure**

Run: `cd /home/benson/gateway && go test ./internal/gatewayws/ -run TestChunkTracker`
Expected: build failure — `chunkTracker` undefined.

- [ ] **Step 3: Implement the tracker**

Create `internal/gatewayws/chunk_tracker.go`:

```go
package gatewayws

import "sync"

type chunkEntry struct {
	received int32
	expected int32 // 0 until first chunk arrives
}

type chunkTracker struct {
	mu        sync.Mutex
	pending   map[int64]*chunkEntry
	drainedCh chan struct{}
}

func newChunkTracker() *chunkTracker {
	t := &chunkTracker{
		pending:   make(map[int64]*chunkEntry),
		drainedCh: make(chan struct{}, 1),
	}
	t.signalIfEmptyLocked() // start drained
	return t
}

// registerRequest marks the guild as awaiting chunks. Idempotent.
func (t *chunkTracker) registerRequest(guildID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.pending[guildID]; ok {
		return
	}
	t.pending[guildID] = &chunkEntry{}
	// Drain into the channel so drained() blocks until a chunk arrives
	// AND the count reaches zero.
	select {
	case <-t.drainedCh:
	default:
	}
}

// recordChunk applies a chunk arrival. Unregistered guilds are ignored
// (a request may have been made before this tracker existed, or after a
// reconnect reset state). expected==0 means we never request — skip.
func (t *chunkTracker) recordChunk(guildID int64, index, count int32) {
	if count <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.pending[guildID]
	if !ok {
		return
	}
	e.received++
	if count > e.expected {
		e.expected = count
	}
	if e.received >= e.expected {
		delete(t.pending, guildID)
		t.signalIfEmptyLocked()
	}
}

// drained returns a channel that fires (receivable) when there are zero
// pending guilds. Subsequent registerRequest calls re-empty the channel
// so a caller that re-arms will block again.
func (t *chunkTracker) drained() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.signalIfEmptyLocked()
	return t.drainedCh
}

func (t *chunkTracker) pendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

func (t *chunkTracker) signalIfEmptyLocked() {
	if len(t.pending) == 0 {
		select {
		case t.drainedCh <- struct{}{}:
		default:
		}
	}
}
```

- [ ] **Step 4: Run tests to confirm pass**

Run: `cd /home/benson/gateway && go test ./internal/gatewayws/ -run TestChunkTracker -v`
Expected: PASS for all four subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/gatewayws/chunk_tracker.go internal/gatewayws/chunk_tracker_test.go
git commit -m "gateway: add per-session chunk drain tracker"
```

---

### Task 6: Wire tracker into Session

**Files:**
- Modify: `internal/gatewayws/ws.go` (Session struct, NewSession, event loop)
- Modify: `internal/gatewayws/write.go:187-199`

- [ ] **Step 1: Add the field and init**

In `internal/gatewayws/ws.go`, add to the `Session` struct near the other Phase-2-related fields (after `divergenceRatio`):

```go
chunks *chunkTracker
```

In `NewSession`, initialize after the other field assignments:

```go
sess.chunks = newChunkTracker()
```

- [ ] **Step 2: Register requests**

In `internal/gatewayws/write.go`, modify `requestGuildMembers`:

```go
func (s *Session) requestGuildMembers(guild int64) {
	select {
	case s.wch <- &Op{
		Op: 8,
		D: RequestGuildMembers{
			GuildID: guild,
		},
	}:
		if s.chunks != nil {
			s.chunks.registerRequest(guild)
		}
	default:
		s.log.Error(s.ctx, "write channel full")
	}
}
```

(Register only on a successful enqueue. A dropped send means no chunks coming.)

- [ ] **Step 3: Record chunks in the event loop**

In `internal/gatewayws/ws.go`, just after the `evtPayload, err := s.state.HandleEvent(ctx, ev)` block (after the `if err != nil { continue }` arm), add:

```go
if ev.T == "GUILD_MEMBERS_CHUNK" && evtPayload != nil && s.chunks != nil {
    s.chunks.recordChunk(evtPayload.GuildID, evtPayload.MemberChunkIndex, evtPayload.MemberChunkCount)
}
```

Place it before `s.pushEventToRedis(ev)` so the tracker advances even if the Redis push later errors.

- [ ] **Step 4: Build + run tests**

Run: `cd /home/benson/gateway && go build ./... && go test ./internal/gatewayws/ ./handler/ ./discord/...`
Expected: success / PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gatewayws/ws.go internal/gatewayws/write.go
git commit -m "gateway: wire chunk tracker into request + event loop"
```

---

### Task 7: Replace fixed stabilize sleep with drain wait

**Files:**
- Modify: `internal/gatewayws/ws.go:629-669` (`holdStabilize`)
- Modify: `internal/gatewayws/ws.go` (SessionConfig + NewSession to accept `StabilizeUseDrain`)
- Modify: `internal/manager/manager.go` (env read + propagation)

- [ ] **Step 1: Add the toggle to SessionConfig + Session**

In `internal/gatewayws/ws.go` SessionConfig:

```go
// StabilizeUseDrain switches holdStabilize from a fixed-duration sleep
// (Phase 1) to a per-shard chunk-drain wait (Phase 2). The fixed
// duration becomes a soft floor; StabilizeMaxDuration remains the hard
// cap.
StabilizeUseDrain bool
```

In `Session`:

```go
stabilizeUseDrain bool
```

In `NewSession` assignments:

```go
stabilizeUseDrain: cfg.StabilizeUseDrain,
```

- [ ] **Step 2: Replace `holdStabilize`**

```go
func (s *Session) holdStabilize() {
	if s.stabilizeSem == nil || s.stabilizeDuration <= 0 {
		return
	}

	max := s.stabilizeMaxDuration
	if max <= 0 {
		max = 2 * s.stabilizeDuration
	}

	acquireStart := time.Now()
	select {
	case s.stabilizeSem <- struct{}{}:
	case <-s.ctx.Done():
		return
	}
	acquired := time.Now()
	s.log.Info(s.ctx, "stabilize semaphore acquired",
		slog.F("wait", acquired.Sub(acquireStart).String()))

	defer func() {
		select {
		case <-s.stabilizeSem:
		default:
		}
		s.log.Info(s.ctx, "stabilize semaphore released",
			slog.F("held", time.Since(acquired).String()),
			slog.F("pending_at_release", s.chunks.pendingCount()))
	}()

	if !s.stabilizeUseDrain {
		hold := min(s.stabilizeDuration, max)
		t := time.NewTimer(hold)
		defer t.Stop()
		select {
		case <-t.C:
		case <-s.ctx.Done():
		}
		return
	}

	// Phase 2: drain wait, bounded by max; minimum hold = stabilizeDuration.
	floor := time.NewTimer(s.stabilizeDuration)
	defer floor.Stop()
	cap := time.NewTimer(max)
	defer cap.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-cap.C:
			s.log.Warn(s.ctx, "stabilize hit max-duration cap before drain",
				slog.F("pending", s.chunks.pendingCount()))
			return
		case <-s.chunks.drained():
			// All known chunks landed. Wait out the floor (if any
			// remaining) before releasing; if the floor already fired,
			// return immediately.
			select {
			case <-floor.C:
				return
			case <-s.ctx.Done():
				return
			case <-cap.C:
				return
			}
		}
	}
}
```

- [ ] **Step 3: Wire the env knobs in manager**

In `internal/manager/manager.go`, replace the existing stabilize env reads with:

```go
stabilizeUseDrain := envBool(cfg.Logger, ctx, "IDENTIFY_STABILIZE_USE_DRAIN", true)

// Default for the floor depends on whether drain is enabled. With drain
// on, the drain signal does the gating; the floor only matters for
// shards that have nothing to drain, so a small value is enough. With
// drain off (rollback), restore Phase 1's 60s fixed-hold semantics.
defaultStabilize := 60 * time.Second
if stabilizeUseDrain {
    defaultStabilize = 5 * time.Second
}
stabilizeDuration := envDuration(cfg.Logger, ctx, "IDENTIFY_STABILIZE_SECONDS", defaultStabilize)

// Cap is independent of the floor — previously coupled as 2× by
// convention. Matches Phase 1's 120s default; only fires on stalled
// drains, so lowering it gains no speedup.
stabilizeMax := envDuration(cfg.Logger, ctx, "IDENTIFY_STABILIZE_SECONDS_MAX", 120*time.Second)
```

(If `envBool` does not exist, copy the pattern from `envInt` / `envDuration` in the same file.)

Pass `StabilizeUseDrain: stabilizeUseDrain` through `SessionConfig` for each shard (alongside the existing `StabilizeDuration` and `StabilizeMaxDuration` fields).

Update the startup log line that records stabilize tuning to include the new fields:

```go
cfg.Logger.Info(ctx, "stabilize gate configured",
    slog.F("stabilize_use_drain", stabilizeUseDrain),
    slog.F("stabilize_concurrency", stabilizeConcurrency),
    slog.F("stabilize_floor", stabilizeDuration.String()),
    slog.F("stabilize_max", stabilizeMax.String()),
)
```

(Match field names to the existing log call site; rename `stabilize_duration` to `stabilize_floor` to reflect the new semantics.)

**Also update the Phase-1 fallback in `gatewayws.NewSession`**: at `internal/gatewayws/ws.go:186-189`, the existing code defaults `StabilizeMaxDuration` to `2 × StabilizeDuration` when zero. That coupling needs to go — leave the fallback for backwards compat (in case `StabilizeMaxDuration` is unset by a caller), but the manager always passes both explicitly now, so the fallback is dead code in practice. Leave it as a safety, no change required.

- [ ] **Step 4: Build + tests**

Run: `cd /home/benson/gateway && go build ./... && go test ./...`
Expected: success / PASS. (Existing tests in `discord/discordjson`, `discord/`, and the new tracker tests pass; nothing else added.)

- [ ] **Step 5: Manual smoke check (no run required, but eyeball the wiring)**

Confirm in `ws.go` that `holdStabilize` is still called only when `s.shouldProcessMembers()` (line ~559) and that the tracker is initialised in `NewSession` even when stabilize is disabled (so `recordChunk` calls are always safe).

- [ ] **Step 6: Commit**

```bash
git add internal/gatewayws/ws.go internal/manager/manager.go
git commit -m "gateway: drain-based stabilize wait with floor + max cap"
```

---

### Task 8: Document Phase 2 in the spec

**Files:**
- Modify: `docs/superpowers/specs/2026-04-30-faster-mass-reconnect-design.md` (Status line + Phasing section)

- [ ] **Step 1: Update status + Phase 2 status note**

Edit the Status line at the top of the spec from `Status: Design` to `Status: Phase 1 + Phase 2 + Phase 3 shipped`.

Append a short note to the Phase 2 paragraph in the Phasing section pointing at the implementing branch / commit (fill in commit hash once merged):

```
Implemented on `feat/faster-mass-reconnect-phase1` (see commits up to the Phase 2 drain-wait change). Knob `IDENTIFY_STABILIZE_USE_DRAIN=true` enables drain-based wait; set to `false` to fall back to Phase 1 fixed sleep during rollout.
```

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-04-30-faster-mass-reconnect-design.md
git commit -m "docs: mark Phase 2 (drain-based stabilize) shipped"
```

---

## Self-review checklist (to run before finishing the plan)

- [ ] Decoders both populate `ChunkIndex`/`ChunkCount` — verified by JSON test; ETF verified by build + manual review of helper choice.
- [ ] Tracker is safe to call with `chunks == nil` (we always initialise, but defensive guards in the call sites are present).
- [ ] `holdStabilize` is still no-op when `stabilizeSem == nil || stabilizeDuration <= 0`.
- [ ] `stabilizeUseDrain=false` reproduces Phase 1 behaviour exactly (rollback path).
- [ ] Max-duration cap fires regardless of drain — verified by code inspection of the inner `select`.
- [ ] No new external dependencies, no schema changes.

## Follow-ups (not part of this plan)

- Fix `pushEventToRedis` skip string mismatch (`GUILD_MEMBER_CHUNK` → `GUILD_MEMBERS_CHUNK`) at `internal/gatewayws/ws.go:466` — separate commit, separate PR.
- Add the metrics enumerated in the spec (`gateway_backfill_chunks_received_total`, `gateway_backfill_drain_seconds`, etc.) once the metrics layer is in place. The tracker has the data; the hook points are obvious (count++ on `recordChunk`, timer at `registerRequest → drained`).
- Optional batcher-queue-depth signal as an AND condition with drain. Defer until observability shows it's needed.
- Measure the cost of `GetGuildMemberCount` (`SELECT count(*) FROM members WHERE guild_id = $1`) under a divergence-enabled mass reconnect. The PK `(guild_id, user_id)` already enables an index-only scan, so per-guild cost is bounded by members-in-guild. If aggregate cost during a 1024-shard storm is too high, the fix is a maintained "rows-in-DB" counter on `guilds` — separate from the existing Discord-count copy, fed by `INSERT … RETURNING (xmax = 0)` from `processMemberBatch` and matching delete-path accounting. Do not introduce it speculatively; measure first.
