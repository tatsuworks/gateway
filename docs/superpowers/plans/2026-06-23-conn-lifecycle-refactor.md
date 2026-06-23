# Connection-Lifecycle Refactor (`Session` → `Session` + `conn`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the reused-across-reconnects `gatewayws.Session` god-object into a durable `Session` (identity + deps + resume tuple) and a fresh-per-`Open()` `conn` (connection-scoped channels, sockets, goroutines, timing), so the bug class we keep patching (captured-channel orphaning, heartbeat reset loops, `lastHB`/`lastAck` data races) becomes structurally impossible.

**Architecture:** Today one mutable `Session` is reused for every reconnect; `Open()` manually resets connection state (`resetHeartbeat`, `wch`/`prioch` swaps, `s.wch = make(...)` on INVALID_SESSION) and four goroutines (`writer`, `sendHeartbeats`, read loop, `logTotalEvents`) mutate shared struct fields with no synchronization. After this refactor, each `Open()` constructs a brand-new `conn` that *owns* its ctx/cancel, channels, websocket, buffers and timing state; the `Session` keeps only durable identity, shared dependencies, and the resume tuple, plus an `atomic.Pointer[conn]` to the current connection so management RPCs (`Cancel`/`ForceIdentify`/`RequestGuildMembers`/`Status`/`LongLastAck`) route to it. A connection cannot inherit stale channels or timestamps because they did not exist a moment earlier.

**Tech Stack:** Go 1.25, `nhooyr.io/websocket`, `golang.org/x/time/rate`, `cdr.dev/slog`, etcd `concurrency`, `czlib`. Tests use `slogtest` and the standard `testing` package; the race detector is the primary regression gate.

## Global Constraints

- **Behavior-preserving.** This is a pure restructuring. No protocol behavior, timing, log message, or persisted-state change is in scope except the synchronization fixes called out in Task 5. Discord-facing behavior must be identical.
- **External API frozen.** These exported symbols and signatures must not change — they are consumed by `internal/manager`: `gatewayws.NewSession(*SessionConfig) (*Session, error)`, `(*Session).Open(ctx context.Context, token string) error`, `(*Session).ForceIdentify()`, `(*Session).Cancel()`, `(*Session).RequestGuildMembers(guildID int64)`, `(*Session).Status() string`, `(*Session).LongLastAck(threshold time.Duration) bool`. Also frozen: `Intents`, `SessionConfig`, the `Intent*` consts, `LargeThreshold`.
- **Green gate (run from `repos/gateway`, every commit):**
  ```bash
  GOCACHE=/tmp/go-build go build ./... \
    && GOCACHE=/tmp/go-build go vet ./internal/gatewayws/ ./internal/manager/ \
    && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ ./internal/manager/
  ```
- **Go version floor:** `go 1.25` (per `go.mod`) — `atomic.Pointer[T]` and `atomic.Bool` are available; use them, not `unsafe`/`atomic.Value`.
- **Resume tuple stays single-owner.** `seq` (atomic int64), `sessID`, `resumeURL`, `last`, `forceIdentify` remain on `Session`. Mutation discipline: `seq`/`forceIdentify`/**`last`** via `sync/atomic`; `sessID`/`resumeURL` mutated only on the read-loop / `Open` goroutine. Do not move these onto `conn`. (`last` MUST become atomic — see Design Decision 8; it is written from the `logTotalEvents` goroutine today, so leaving it a bare field is a race.)

---

## Design Decisions (read before starting)

These were chosen deliberately; flag the maintainer if you disagree before coding.

1. **`conn` holds a back-reference `s *Session`** for durable deps (logger, encoder, DB, redis, etcd, rate limiter, buffer pool) and the resume tuple. Connection methods read deps via `c.s.X`. This avoids copying deps and keeps one source of truth.
2. **`rl *rate.Limiter` stays on `Session` (shared across reconnects).** Today the limiter persists between connections; making it per-`conn` would reset the send budget on every reconnect — a behavior change. Keep it on `Session`; `writer` calls `c.s.rl.Wait(c.ctx)`.
3. **`bufferPool` stays on `Session`; the in-flight `buf` moves to `conn`.** The pool is a shared dependency; the borrowed buffer is connection-scoped.
4. **Timing/state fields (`lastHB`, `lastAck`, `ready`, `curState`) move to `conn` and are guarded by a `conn.mu sync.Mutex`** with `setState`/`snapshot` accessors. `authed` moves to `conn` as an `atomic.Bool` (it is read on the hot writer path). This subsumes the pre-existing `lastAck`/`curState` data races (review finding #3) — but that hardening is isolated to Task 5 so the receiver migration (Task 2–4) stays mechanical.
5. **`Open()` becomes a thin wrapper**: build ctx, build `conn`, publish it to `s.cur`, run, clear `s.cur` on return. The former `Open()` body becomes `(*conn).run(token string) error`.
6. **`resetHeartbeat` is deleted** (a fresh `conn` has zero-valued `lastHB`/`lastAck`), and the INVALID_SESSION `s.wch = make(chan *Op, 2000)` line and the identify-path channel-swap block are deleted — all three are reconnect-reset hacks that a fresh-per-connection object makes unnecessary.
7. **Management routing tolerates a nil current connection** (a `Cancel`/`Status` can land before the first `Open` or between reconnects). Routing is via `s.cur.Load()` with a nil check.
8. **`last` stays on `Session` as an atomic int64** (codex review #1). It is the event-rate baseline read AND written by the `logTotalEvents` goroutine (`log.go:30`/`log.go:43`) and reset by `Open` on identify (`ws.go:316`). Today it is an unsynchronized bare field — a latent race. Access it via `atomic.LoadInt64`/`atomic.StoreInt64` everywhere. Keeping it on `Session` (not `conn`) preserves the current resume-carryover: on RESUME the baseline is *not* reset, so the first post-resume "event report" stays accurate.
9. **`guilds` and `backfilled` stay on `Session`, NOT `conn`** (codex review #2). They are mutated and read only on the read-loop goroutine (`READY` handler repopulates them; `GUILD_CREATE`/`isBackfilled` reads them) — single-owner, race-free. Critically, today both **survive a RESUME** (the `RESUMED` handler does not touch them), so the backfill-skip optimization (the 2026-06-18 RGM-skip feature) keeps working across resumes. Moving them to a fresh `conn` would empty them on every resume and re-trigger RGM/reconcile for already-backfilled guilds — a reconnect-storm regression. They are NOT in the Task 5 mutex block (single-owner needs no lock).
10. **`run()` takes the parent context; the two-context split is preserved** (codex review #4). Today `Open` derives a *connection* context (`s.ctx`) for reads/writes/heartbeats/redis, but passes the *parent* (manager) context into `state.HandleEvent` (`ws.go:354`) and `maybeRequestGuildMembers` (`ws.go:367`) so in-flight DB work is NOT aborted by a disconnect. `(*conn).run(parent context.Context)` keeps `c.ctx` for connection-scoped work and threads `parent` into exactly those two calls. Do not switch them to `c.ctx`.
11. **Two accepted, documented behavior changes** (codex review #5/#6 — blessed, not silent): (a) `RequestGuildMembers` via gRPC is *dropped with a log line* when there is no active connection, instead of best-effort buffering on a durable `wch`. It is a management RPC, the socket is down so it cannot be honored promptly anyway, and today's "durability" only held on the resume path — the very identify-path channel swap we are deleting already discarded it. (b) `Status()` returns `"<disconnected>"` and `LongLastAck()` returns `true` when `cur == nil` (between reconnects), rather than echoing the previous connection's stale `curState`/`lastAck`. This is strictly more informative for health monitoring; `LongLastAck`→`true` matches today's "stale ack ⇒ reported unhealthy" outcome.
12. **Concurrent `Session.Open` is unsupported** (codex optional). The manager runs one serial `Open` loop per shard (`manager.go:182-198`), so this holds in production. We rely on it: a stale `Open`'s `defer s.cur.Store(nil)` could otherwise clear a newer conn. Documented as an invariant; not guarded.

### Field partition

| Field | Destination | Notes |
|---|---|---|
| `name`, `log`, `token`, `intents`, `shardID`, `shardCount`, `hasGuildMembersIntent` | `Session` | immutable identity |
| `wg`, `bufferPool`, `enc`, `etcd`, `state`, `stateDB`, `rc`, `whitelistedEvents`, `rl` | `Session` | shared deps |
| `seq`, `sessID`, `resumeURL`, `forceIdentify`, `lastIdentify` | `Session` | resume tuple (single-owner) |
| `last` | `Session` (**atomic int64**) | event-rate baseline; written by `logTotalEvents` goroutine → must be atomic (Decision 8) |
| `guilds`, `backfilled` | `Session` | read-loop single-owner; **survive RESUME** — keeping them here preserves backfill-skip (Decision 9) |
| `cur atomic.Pointer[conn]` | `Session` | **new** — current connection |
| `ctx`, `cancel` | `conn` | per-connection lifetime |
| `wsConn`, `zr`, `interval`, `trace` | `conn` | per-connection transport |
| `wch`, `prioch` | `conn` | **the bug source** — per-connection |
| `buf` | `conn` | borrowed per read |
| `etcdSess`, `identifyMu` | `conn` | created per `Open` in `initEtcd` |
| `authed` | `conn` (`atomic.Bool`) | hot-path read in `writer` |
| `lastHB`, `lastAck`, `ready`, `curState` | `conn` (under `conn.mu`) | Task 5 hardening |

### Method-receiver migration map

All of these change receiver from `(s *Session)` to `(c *conn)` and have their bodies rewritten so `s.X` becomes `c.X` (connection field), `c.s.X` (durable dep / resume tuple), or `c.s.method()` (durable method). Durable methods that touch only `Session` fields stay on `Session`.

| Stays on `Session` | Moves to `conn` |
|---|---|
| `Status`, `LongLastAck` (route via `s.cur`) | `writer`, `writeOp`, `writeIdentify`, `writeResume`, `writeHeartbeat`, `heartbeatStale`, `sendHeartbeats`, `requestGuildMembers`, `rotateStatuses` |
| `Cancel`, `ForceIdentify` (route via `s.cur`) | `readMessage`, `readHello`, `readAndDecodeEvent`, `cleanupBuffer` |
| `RequestGuildMembers` (route via `s.cur`) | `handleInternalEvent`, `pushEventToRedis`, `maybeRequestGuildMembers`, `isBackfilled` |
| `NewSession`, `shouldResume`, `shouldProcessMembers`, `calcIdentifyWait` | `GatewayURL`, `initEtcd`, `acquireIdentifyLock`, `releaseIdentifyLock`, `logTotalEvents` |
| `applyForceIdentify`, `persistShardInfo`, `persistStatus`, `loadSeq`, `loadSessID`, `loadResumeURL` | `Open`-body → `(*conn).run` |
| `resetHeartbeat` | **DELETED** |

Notes:
- `shouldProcessMembers`/`calcIdentifyWait` read only intents/`shardID` → stay on `Session`; conn calls `c.s.shouldProcessMembers()`.
- `maybeRequestGuildMembers`/`isBackfilled` move to `conn` but read `c.s.backfilled` (stays on Session per Decision 9) and `c.s.stateDB`.
- `handleInternalEvent` (conn method) repopulates `c.s.guilds`/`c.s.backfilled` in the READY branch and reads `c.s.backfilled` discipline is unchanged (read-loop single-owner).
- `persistShardInfo`/`applyForceIdentify` mutate only resume-tuple fields → stay on `Session`, called via `c.s.`. Their slog calls currently use `s.ctx`; since `s.ctx` is removed, switch those log calls to `context.Background()` (they already use `context.Background()` for their DB calls). Same for `loadSeq`/`loadSessID`/`loadResumeURL` (Decision/codex #3).
- `persistStatus` reads `curState` which now lives on `conn`. Change its signature to `func (s *Session) persistStatus(state string)`; the only caller is `logTotalEvents`, which passes `c.state()` (the mutex-guarded read added in Task 5; pre-Task-5 it passes `c.curState`). Its slog call also moves to `context.Background()`.
- `GatewayURL` reads `c.s.resumeURL`/`c.s.sessID` (Session) and `c.s.enc` → on `conn`.

---

## Task 1: Establish the green baseline and lock the invariants in tests

**Files:**
- Test: `internal/gatewayws/conn_lifecycle_test.go` (Create)

**Interfaces:**
- Produces: nothing structural — this task records the starting state and writes the acceptance tests the refactor must satisfy. The tests reference symbols that exist today (`Session`, channels) and are rewritten to `conn` in Task 4 once the type exists; for now they pin *current* behavior so we can prove the refactor preserved it.

- [ ] **Step 1: Run the existing suite under the race detector and record it is green**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ ./internal/manager/ -v 2>&1 | tail -30
```
Expected: `ok  github.com/tatsuworks/gateway/internal/gatewayws` and `ok  .../internal/manager` (or `no test files` for manager). If anything fails, STOP — fix the baseline before refactoring (per repo AGENTS.md: never stack work on a broken baseline).

- [ ] **Step 2: Write the channel-independence acceptance test (currently un-writable against `Session`; defer the body)**

Add a placeholder test that documents the invariant and skips, so it is tracked but does not block the baseline:

```go
package gatewayws

import "testing"

// After the conn refactor, two sequential connections must not share send
// channels: a fresh conn allocates its own wch/prioch, so a producer for
// connection B can never reach connection A's writer (the captured-locals
// orphaning bug). Re-enabled with a real body in Task 4 once conn exists.
func TestSequentialConnsDoNotShareChannels(t *testing.T) {
	t.Skip("enabled in Task 4 after conn type exists")
}
```

- [ ] **Step 3: Verify the skipped test compiles and the suite is still green**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ -run TestSequentialConnsDoNotShareChannels -v
```
Expected: `--- SKIP: TestSequentialConnsDoNotShareChannels`, package `ok`.

- [ ] **Step 4: Commit**

```bash
git add internal/gatewayws/conn_lifecycle_test.go
git commit -m "test(gatewayws): record green baseline + pin conn-refactor invariants"
```

---

## Task 2: Introduce the `conn` type and migrate all connection-scoped methods to it

This is the structural core. **It is a single Go type migration: the package will not compile partway through — that is expected. Do not run the green gate or commit until the final step.** Work in the field/method order below to minimize churn.

**Files:**
- Create: `internal/gatewayws/conn.go`
- Modify: `internal/gatewayws/ws.go` (`Session` struct, `NewSession`, `Open`, `handleInternalEvent`, `pushEventToRedis`, `GatewayURL`, `initEtcd`, `acquireIdentifyLock`, `releaseIdentifyLock`, `readAndDecodeEvent`, `cleanupBuffer`, `Cancel`, `ForceIdentify`, `Status`, `LongLastAck`, delete `resetHeartbeat`)
- Modify: `internal/gatewayws/write.go` (all `writer`/`write*`/`heartbeatStale`/`sendHeartbeats`/`requestGuildMembers`/`rotateStatuses`/`RequestGuildMembers` receivers)
- Modify: `internal/gatewayws/read.go`, `internal/gatewayws/hello.go` (`readMessage`, `readHello` receivers)
- Modify: `internal/gatewayws/backfill.go` (`maybeRequestGuildMembers`, `isBackfilled` receivers)
- Modify: `internal/gatewayws/log.go` (`logTotalEvents` receiver)

**Interfaces:**
- Produces (new `conn.go`):
  ```go
  type conn struct {
      s *Session

      ctx    context.Context
      cancel context.CancelFunc

      wsConn *websocket.Conn
      zr     io.ReadCloser

      interval time.Duration
      trace    string

      wch    chan *Op
      prioch chan *Op

      buf *bytes.Buffer

      etcdSess   *concurrency.Session
      identifyMu *concurrency.Mutex

      authed atomic.Bool

      // Task 5 will guard these with mu; declared here so receivers compile now.
      mu       sync.Mutex
      lastHB   time.Time
      lastAck  time.Time
      ready    time.Time
      curState string
  }

  func (s *Session) newConn(ctx context.Context, cancel context.CancelFunc) *conn
  func (c *conn) run(token string) error          // former Open body
  ```
- Produces (on `Session`): `cur atomic.Pointer[conn]`; `Open`/`Cancel`/`ForceIdentify`/`Status`/`LongLastAck`/`RequestGuildMembers` route through `cur`.
- Consumes: Task 1 baseline.

- [ ] **Step 1: Create `conn.go` with the struct above and `newConn`**

```go
package gatewayws

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/etcd-io/etcd/clientv3/concurrency"
	"nhooyr.io/websocket"
)

// conn is a single gateway connection. A fresh conn is built per Open and
// discarded when that connection dies, so connection-scoped state (channels,
// socket, heartbeat timing) can never leak across reconnects.
type conn struct {
	s *Session

	ctx    context.Context
	cancel context.CancelFunc

	wsConn *websocket.Conn
	zr     io.ReadCloser

	interval time.Duration
	trace    string

	wch    chan *Op
	prioch chan *Op

	buf *bytes.Buffer

	guilds     map[int64]struct{}
	backfilled map[int64]struct{}

	etcdSess   *concurrency.Session
	identifyMu *concurrency.Mutex

	authed atomic.Bool

	mu       sync.Mutex
	lastHB   time.Time
	lastAck  time.Time
	ready    time.Time
	curState string
}

func (s *Session) newConn(ctx context.Context, cancel context.CancelFunc) *conn {
	return &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		wch:    make(chan *Op, 2000),
		prioch: make(chan *Op),
	}
}
```

- [ ] **Step 2: Trim the `Session` struct in `ws.go`**

Remove these fields from `Session`: `ctx`, `cancel`, `wsConn`, `zr`, `interval`, `trace`, `wch`, `prioch`, `lastHB`, `lastAck`, `ready`, `curState`, `buf`, `authed`, `etcdSess`, `identifyMu`. Add `cur atomic.Pointer[conn]`. **Keep** `guilds`, `backfilled` (Decision 9) and `last` (Decision 8) on `Session`. Keep everything else in the "Session" rows of the field-partition table. Drop the now-unused `bytes`/`io`/`websocket` imports from `ws.go` only if nothing else there uses them (the read/decode helpers move to conn, so re-check after Step 6).

- [ ] **Step 3: In `NewSession`, drop the connection-field initializers**

Remove `ctx: context.Background()`, `wch: make(...)`, `prioch: make(...)` from the `&Session{...}` literal (channels now belong to `conn`; `rl` stays). Everything else in `NewSession` (loadSessID/loadSeq/loadResumeURL, intent scan) is unchanged.

- [ ] **Step 4: Convert `write.go` receivers to `(c *conn)`**

For every method in the "Moves to conn" list that lives in `write.go` (`writer`, `writeOp`, `writeIdentify`, `writeResume`, `writeHeartbeat`, `heartbeatStale`, `sendHeartbeats`, `requestGuildMembers`, `rotateStatuses`), change `func (s *Session)` → `func (c *conn)` and rewrite field references by this rule:
  - connection field (`ctx`, `wch`, `prioch`, `wsConn`, `interval`, `lastHB`, `lastAck`) → `c.X`
  - durable dep / resume tuple (`rl`, `log`, `enc`, `seq`, `token`, `sessID`, `intents`, `shardID`, `shardCount`) → `c.s.X`
  - `s.authed` → `c.authed.Load()` (writer) — the bare-bool read becomes the atomic load
  - `s.cancel` inside `sendHeartbeats` → `c.cancel`
  - `s.Cancel()` inside `writer`'s `defer` → `c.cancel()` (cancel *this* connection directly; no nil-guard needed — a conn always has a cancel)

Delete the now-unused `resetHeartbeat` (it is in `write.go`).

Convert the **internal** `requestGuildMembers(guild int64)` (the `default:`-drop variant) to `(c *conn)`. Leave the **exported** `RequestGuildMembers` migration for Step 7.

- [ ] **Step 5: Convert `read.go`, `hello.go`, `backfill.go`, `log.go` receivers**

Mechanically apply the same receiver swap and field rule to: `readMessage`, `readHello` (read `c.wsConn`, `c.zr`, `c.buf`, `c.s.bufferPool`, `c.s.enc`, sets `c.interval`/`c.trace`); `readAndDecodeEvent`, `cleanupBuffer` (move these two from `ws.go` to conn methods — they touch `buf`/`bufferPool`/`wsConn`; `cleanupBuffer` reads `c.buf`, `c.s.bufferPool`, `c.s.log`); `maybeRequestGuildMembers`, `isBackfilled` (read `c.s.backfilled` — stays on Session per Decision 9 — call `c.requestGuildMembers`, `c.s.stateDB`); `logTotalEvents` (reads `c.ctx`, `c.s.seq` atomic, `c.s.last` **via `atomic.LoadInt64`**, `len(c.wch)`, `c.s.state.WaitingQueries()`, `c.curState`, writes `c.s.last` **via `atomic.StoreInt64`**, calls `c.s.persistStatus(c.curState)` — see Step 8 note).

- [ ] **Step 6: Convert the remaining `ws.go` connection methods to `(c *conn)`**

`GatewayURL` (reads `c.s.enc`, `c.s.resumeURL`, `c.s.sessID`), `initEtcd` (reads `c.s.etcd`, `c.s.calcIdentifyWait`, `c.ctx`; sets `c.etcdSess`, `c.identifyMu`), `acquireIdentifyLock`/`releaseIdentifyLock` (read `c.identifyMu`, `c.ctx`, `c.s.log`), `handleInternalEvent` (reads/writes `c.s.seq` atomic, `c.s.sessID`, `c.s.resumeURL`, `c.s.guilds`, `c.s.backfilled` — both stay on Session per Decision 9 — `c.authed`, `c.lastAck`, `c.ready`, `c.identifyMu`, calls `c.s.persistShardInfo`, `c.releaseIdentifyLock`, `c.writeHeartbeat`, `c.s.calcIdentifyWait`, `c.s.stateDB`, `c.s.enc`). In the READY delayed-unlock goroutine, use a local `if err := c.releaseIdentifyLock(); err != nil { ... }` rather than assigning the outer `err` (codex optional cleanup; avoids a closure foot-gun), `pushEventToRedis` (reads `c.s.whitelistedEvents`, `c.s.rc`, `c.s.log`, `c.ctx`).

In `handleInternalEvent` case 9 (INVALID_SESSION): **delete the `s.wch = make(chan *Op, 2000)` line** — the next `Open` builds a fresh conn with fresh channels. Keep the rest of case 9.

- [ ] **Step 7: Rewrite `Open` as a wrapper + move its body into `(*conn).run`**

Replace `Open` and add `run`:

```go
func (s *Session) Open(ctx context.Context, token string) error {
	s.wg.Add(1)
	defer s.wg.Done()

	cctx, cancel := context.WithCancel(ctx)

	c := s.newConn(cctx, cancel)
	s.cur.Store(c)
	// Cancel the connection's goroutines BEFORE retracting it from cur, so a
	// concurrent Cancel/ForceIdentify never sees a nil cur while this
	// connection's context is still live (codex review #7). Single combined
	// defer guarantees the order regardless of LIFO.
	defer func() {
		cancel()
		s.cur.Store(nil)
	}()

	return c.run(ctx) // pass the PARENT ctx; see two-context note below
}
```

`(*conn).run(parent context.Context) error` is the former `Open` body with these edits:
  - signature is `func (c *conn) run(parent context.Context) error`. `Open`'s own `token` parameter stays unused exactly as today (the body reads `c.s.token`); do not thread it into `run`. Confirm by grep that `Open`'s `token` param is unused.
  - drop `s.wg.Add/Done` and the ctx creation (now in `Open`);
  - drop `defer func(){ s.authed = false }()` → `defer c.authed.Store(false)`;
  - drop `s.resetHeartbeat()` (fresh conn);
  - drop the identify-path channel-swap block entirely (the `if !s.shouldResume() && len(...)>0 { ... }`) — fresh conn already has empty channels;
  - `s.last = 0` (the identify-path reset) → `atomic.StoreInt64(&c.s.last, 0)` (Decision 8);
  - `s.applyForceIdentify()` → `c.s.applyForceIdentify()`; `s.shouldResume()` → `c.s.shouldResume()`; `s.initEtcd()`/`acquireIdentifyLock()` → `c.initEtcd()`/`c.acquireIdentifyLock()`;
  - `go s.writer()` → `go c.writer()`, etc. for `sendHeartbeats`, `logTotalEvents`;
  - `s.curState = "..."` → `c.curState = "..."` (Task 5 swaps these for `c.setState(...)`);
  - **Two-context rule (codex review #4 — behavior-critical):** in the read loop, connection-scoped work uses `c.ctx` (`readAndDecodeEvent`, `writeHeartbeat`, `pushEventToRedis`, all already routed via `c.ctx`), but the two calls that today receive the *parent* ctx keep receiving `parent`: `c.s.state.HandleEvent(parent, ev)` and `c.maybeRequestGuildMembers(parent, evtPayload)`. Do NOT switch these to `c.ctx` — that would abort in-flight DB writes on disconnect. `maybeRequestGuildMembers`'s `ctx` parameter therefore receives `parent`.
  - remaining read loop `s.X` → `c.X` / `c.s.X` per the rule;
  - `defer s.persistShardInfo()` → `defer c.s.persistShardInfo()`.

- [ ] **Step 8: Resolve `persistStatus`/`persistShardInfo` curState access**

`persistStatus` (in `persistence.go`) reads `s.curState`, which now lives on `conn`. Keep `persistStatus` on `Session` but give it the state as an argument: change to `func (s *Session) persistStatus(state string)` and pass it in — `logTotalEvents` calls `c.s.persistStatus(c.curState)` (Task 5: `c.state()`). `Open`/`run` have no direct `persistStatus` call (only `persistShardInfo`). Verify with grep that `persistStatus` has exactly the call site(s) in `log.go`.

**Compile fix (codex review #3) — `s.ctx` is gone, so the Session-level persistence/load/force methods can no longer log with it.** In `persistShardInfo`, `persistStatus`, `loadSeq`, `loadSessID`, `loadResumeURL` (all `persistence.go`) and `applyForceIdentify` (`ws.go`), change every `s.log.*(s.ctx, ...)` to `s.log.*(context.Background(), ...)`. This is behavior-equivalent: those methods already pass `context.Background()` to their DB calls, and at `NewSession` time `s.ctx` was `context.Background()` anyway. These stay `(*Session)` methods (they touch only resume-tuple fields), called via `c.s.`.

- [ ] **Step 9: Rewire the management methods on `Session`**

```go
func (s *Session) Cancel() {
	if c := s.cur.Load(); c != nil {
		c.cancel()
	}
}

func (s *Session) ForceIdentify() {
	atomic.StoreInt32(&s.forceIdentify, 1)
	s.Cancel()
}

func (s *Session) RequestGuildMembers(guildID int64) {
	c := s.cur.Load()
	if c == nil {
		s.log.Info(context.Background(), "drop members request: no active connection", slog.F("guild", guildID))
		return
	}
	c.requestGuildMembersExternal(guildID)
}
```

Move the exported-`RequestGuildMembers` *body* (the `select { case s.wch <- op: case <-s.ctx.Done(): }` version with the info log) onto `conn` as `requestGuildMembersExternal(guildID int64)`, reading `c.wch`/`c.ctx`/`c.s.log`. (Keep `Status`/`LongLastAck` for Step 10.)

- [ ] **Step 10: Rewire `Status` / `LongLastAck` through `s.cur`**

```go
func (s *Session) Status() string {
	c := s.cur.Load()
	if c == nil {
		return fmt.Sprintf("%v: <disconnected>", s.shardID)
	}
	return fmt.Sprintf("%v: %s [LastAck: %v]", s.shardID, c.curState, c.lastAck.Format(time.RFC3339))
}

func (s *Session) LongLastAck(threshold time.Duration) bool {
	c := s.cur.Load()
	if c == nil {
		// no active connection => definitionally not acking
		return true
	}
	cutoff := time.Now().Add(-threshold)
	return c.lastAck.Before(cutoff) && c.ready.Before(cutoff)
}
```
(Task 5 replaces the raw `c.curState`/`c.lastAck`/`c.ready` reads with a mutex-guarded `c.snapshot()`.)

- [ ] **Step 11: Build, vet, and run the race suite (first compile point)**

Run the full green gate:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go build ./... \
  && GOCACHE=/tmp/go-build go vet ./internal/gatewayws/ ./internal/manager/ \
  && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ ./internal/manager/ 2>&1 | tail -30
```
Expected: builds clean, vet clean, and tests **fail to compile** only in the test files that construct `Session{ctx:..., wch:...}` literals (force_identify_test.go, heartbeat_deadlock_test.go, conn_lifecycle_test.go). Production code must build. If production fails to build, fix before moving on. Do **not** commit yet — test migration is Task 4 and the commit is shared.

> If you prefer a green commit here, temporarily build-tag or `t.Skip` the broken test files, commit production, then un-skip in Task 4. Otherwise carry the uncommitted change into Task 4 and commit once.

---

## Task 3: Migrate the test files to the `conn` type

**Files:**
- Modify: `internal/gatewayws/heartbeat_deadlock_test.go`
- Modify: `internal/gatewayws/force_identify_test.go`
- Modify: `internal/gatewayws/conn_lifecycle_test.go`

**Interfaces:**
- Consumes: the `conn` type and methods from Task 2.
- Produces: a compiling, green `-race` suite proving behavior preserved.

- [ ] **Step 1: Port `heartbeat_deadlock_test.go` to build `conn` literals**

Every `s := &Session{ctx, cancel, log, rl, wch, prioch, prioch, authed, ...}` becomes a `conn` literal. The methods under test (`writer`, `writeHeartbeat`, `writeIdentify`, `writeResume`, `heartbeatStale`, `RequestGuildMembers`) are now conn methods, except `RequestGuildMembers` which is still a `Session` method that routes to the conn. Rewrite, e.g.:

```go
func TestWriterCancelsConnectionWhenItExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{log: slogtest.Make(t, nil), rl: rate.NewLimiter(rate.Every(time.Hour), 0)}
	c := &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		wch:    make(chan *Op, 1),
		prioch: make(chan *Op, 1),
	}
	c.authed.Store(true)
	c.prioch <- &Op{Op: 1}

	done := make(chan struct{})
	go func() { c.writer(); close(done) }()
	// ...assert <-done, then <-c.ctx.Done() as before, replacing s.ctx with c.ctx...
}
```
Apply the analogous change to `TestWriteHeartbeatUnblocksWhenContextCancelled`, `assertUnblocksOnCancel` (build a `conn`, call `run func(*conn)`), `TestWriteIdentifyUnblocksOnCancel`/`TestWriteResumeUnblocksOnCancel` (`(*conn).writeIdentify`/`(*conn).writeResume`), and `TestRequestGuildMembersUnblocksOnCancel`. For the RGM test, because `RequestGuildMembers` now routes through `s.cur`, set it up:
```go
s := &Session{log: slogtest.Make(t, nil)}
c := &conn{s: s, ctx: ctx, cancel: cancel, wch: make(chan *Op), prioch: make(chan *Op)}
s.cur.Store(c)
cancel()
// assert s.RequestGuildMembers(123) returns promptly
```
`TestHeartbeatStale` uses `&Session{lastHB, lastAck, interval}` → `&conn{lastHB:..., lastAck:..., interval:...}` and `s.heartbeatStale(...)` → `c.heartbeatStale(...)`. `TestWriteHeartbeatDeliversWhenDrained` (`heartbeat_deadlock_test.go:172`) → build a `conn` and call `c.writeHeartbeat()` against `c.prioch`.

**`TestResetHeartbeatPreventsStaleCancelAfterReconnect` (`heartbeat_deadlock_test.go:152`) calls the deleted `resetHeartbeat` (codex review #8).** Rewrite it to assert the invariant `resetHeartbeat` used to guarantee, now provided structurally by `newConn`:

```go
func TestFreshConnHasZeroHeartbeatState(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.newConn(ctx, cancel)
	c.interval = 40 * time.Second
	if !c.lastHB.IsZero() || !c.lastAck.IsZero() {
		t.Fatalf("fresh conn carried heartbeat state: lastHB=%v lastAck=%v", c.lastHB, c.lastAck)
	}
	if c.heartbeatStale(time.Now()) {
		t.Fatal("heartbeatStale fired on a fresh connection; would cause a reconnect loop")
	}
}
```

- [ ] **Step 2: Port `force_identify_test.go`**

`newResumableSession` builds a `Session` with the resume tuple (`seq`,`sessID`,`resumeURL`,`shardID`,`name`,`stateDB`,`log`) — those stay on `Session`, so drop only the `ctx`/`cancel` fields from the literal (they no longer exist on `Session`). For tests that assert `ForceIdentify` cancels the active connection, attach a conn: `c := s.newConn(ctx, cancel); s.cur.Store(c)` then assert `<-c.ctx.Done()`. For the "signals without mutating resume state" test, the resume-tuple assertions are unchanged (they read `s.sessID` etc.). Where a test previously relied on `s.ctx`/`s.cancel` directly, route through the conn.

- [ ] **Step 3: Give `conn_lifecycle_test.go` its real body**

Replace the skipped `TestSequentialConnsDoNotShareChannels` with a real assertion that two conns from the same Session have distinct channels:

```go
func TestSequentialConnsDoNotShareChannels(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := s.newConn(ctx, cancel)
	b := s.newConn(ctx, cancel)
	if a.wch == b.wch || a.prioch == b.prioch {
		t.Fatal("two connections share send channels; reconnect can orphan the writer")
	}
}
```

- [ ] **Step 3b: Add acceptance tests for the blessed nil-`cur` behavior (Decision 11)**

These pin the documented behavior changes so they are intentional, not regressions:

```go
// ForceIdentify with no active connection must still take effect on the next
// Open: the atomic flag persists even though Cancel is a no-op.
func TestForceIdentifyWithNilCurStillFlags(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	s.ForceIdentify() // cur is nil
	if atomic.LoadInt32(&s.forceIdentify) != 1 {
		t.Fatal("ForceIdentify did not set the flag when no connection was active")
	}
}

// RequestGuildMembers is dropped (not buffered) when no connection is active.
// It must return promptly without panicking on a nil cur.
func TestRequestGuildMembersDroppedWhenDisconnected(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil)}
	done := make(chan struct{})
	go func() { s.RequestGuildMembers(123); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RequestGuildMembers blocked on a nil cur")
	}
}

// Status/LongLastAck report disconnected when there is no active connection.
func TestStatusWhenDisconnected(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil), shardID: 9}
	if got := s.Status(); got == "" {
		t.Fatal("Status returned empty on nil cur")
	}
	if !s.LongLastAck(time.Minute) {
		t.Fatal("LongLastAck should report true (no acks) on nil cur")
	}
}
```

- [ ] **Step 4: Run the full green gate**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go build ./... \
  && GOCACHE=/tmp/go-build go vet ./internal/gatewayws/ ./internal/manager/ \
  && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ ./internal/manager/ -v 2>&1 | tail -40
```
Expected: build clean, vet clean, all tests PASS under `-race`.

- [ ] **Step 5: Commit (the Task 2 + Task 3 change set together)**

```bash
git add internal/gatewayws/
git commit -m "refactor(gatewayws): split Session into durable Session + per-Open conn

A fresh conn owns ctx/cancel, wch/prioch, the websocket, buffers and
heartbeat timing; Session keeps identity, shared deps and the resume tuple
plus an atomic.Pointer[conn] that management RPCs route through. Removes the
reconnect-reset hacks (resetHeartbeat, identify-path channel swap, the
INVALID_SESSION wch rebuild) whose only purpose was scrubbing a reused
object — a fresh conn cannot inherit stale channels or timestamps."
```

---

## Task 4: Harden the connection-state synchronization (subsumes review finding #3)

**Files:**
- Modify: `internal/gatewayws/conn.go` (add `mu`-guarded accessors)
- Modify: `internal/gatewayws/ws.go` (`Status`, `LongLastAck`, `handleInternalEvent`, `run` state writes)
- Modify: `internal/gatewayws/write.go` (`sendHeartbeats`, `heartbeatStale`)
- Modify: `internal/gatewayws/log.go` (`logTotalEvents` curState read)
- Test: `internal/gatewayws/conn_lifecycle_test.go`

**Interfaces:**
- Produces on `conn`:
  ```go
  func (c *conn) setState(s string)
  func (c *conn) markAck(t time.Time)
  func (c *conn) markHB(t time.Time)
  func (c *conn) markReady(t time.Time)
  func (c *conn) snapshot() (curState string, lastHB, lastAck, ready time.Time)
  ```
  All take/release `c.mu`. `heartbeatStale(now)` reads `lastHB`/`lastAck` under `c.mu` internally.

- [ ] **Step 1: Write a failing race test for concurrent Status during a connection**

```go
func TestStatusRaceWithConnectionWrites(t *testing.T) {
	s := &Session{log: slogtest.Make(t, nil), shardID: 7}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := s.newConn(ctx, cancel)
	s.cur.Store(c)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			c.setState("handle internal event X")
			c.markAck(time.Now())
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = s.Status()
		_ = s.LongLastAck(time.Minute)
	}
	<-done
}
```

- [ ] **Step 2: Run it under -race and watch it fail**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ -run TestStatusRaceWithConnectionWrites -v
```
Expected: `DATA RACE` on `curState`/`lastAck` (raw field access from Step-2-of-Task-2 still unguarded), or compile error if `setState`/`markAck` don't exist yet — either way RED.

- [ ] **Step 3: Add the `mu`-guarded accessors to `conn.go`**

```go
func (c *conn) setState(s string) { c.mu.Lock(); c.curState = s; c.mu.Unlock() }
func (c *conn) markHB(t time.Time)  { c.mu.Lock(); c.lastHB = t; c.mu.Unlock() }
func (c *conn) markAck(t time.Time) { c.mu.Lock(); c.lastAck = t; c.mu.Unlock() }
func (c *conn) markReady(t time.Time) { c.mu.Lock(); c.ready = t; c.mu.Unlock() }

func (c *conn) snapshot() (curState string, lastHB, lastAck, ready time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curState, c.lastHB, c.lastAck, c.ready
}
```

- [ ] **Step 4: Route every state read/write through the accessors**

  - `run`/`handleInternalEvent`/`readAndDecodeEvent`: every `c.curState = "..."` → `c.setState("...")`.
  - case 11 (`HEARTBEAT_ACK`): `c.lastAck = time.Now()` → `c.markAck(time.Now())`.
  - READY/RESUMED/RESUME: `c.ready = time.Now()` → `c.markReady(time.Now())`; `c.authed = true` is already `c.authed.Store(true)`.
  - `sendHeartbeats`: `c.lastHB = time.Now()` → `c.markHB(time.Now())`; the `s.heartbeatStale(...)` log fields use a `snapshot()`.
  - `heartbeatStale(now)`: take `c.mu` and read `lastHB`/`lastAck` inside.
  - `logTotalEvents`: read `curState` via `cur, _, _, _ := c.snapshot()`.
  - `Status`/`LongLastAck`: use `c.snapshot()` instead of raw reads.

- [ ] **Step 5: Run the race test — now green**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ -run TestStatusRaceWithConnectionWrites -v
```
Expected: PASS, no DATA RACE.

- [ ] **Step 6: Full green gate + commit**

```bash
cd repos/gateway && GOCACHE=/tmp/go-build go build ./... \
  && GOCACHE=/tmp/go-build go vet ./internal/gatewayws/ ./internal/manager/ \
  && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ ./internal/manager/
```
Expected: all green.
```bash
git add internal/gatewayws/
git commit -m "refactor(gatewayws): guard conn timing/state behind a mutex

Status/LongLastAck (manager goroutine) and the read-loop/heartbeat goroutines
now read and write lastHB/lastAck/ready/curState through mu-guarded accessors,
closing the pre-existing lastAck/curState data race. authed is an atomic.Bool."
```

---

## Task 5: Whole-repo verification and review

**Files:** none (verification only).

- [ ] **Step 1: Full module build + vet + race across all touched packages**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go build ./... \
  && GOCACHE=/tmp/go-build go vet ./... \
  && GOCACHE=/tmp/go-build go test -race ./internal/gatewayws/ ./internal/manager/ ./gatewaypb/ 2>&1 | tail -40
```
Expected: clean build/vet, all tests PASS.

- [ ] **Step 2: Confirm the external API is byte-for-byte unchanged**

Run:
```bash
cd repos/gateway && GOCACHE=/tmp/go-build go doc ./internal/gatewayws Session 2>&1 | grep -E "func .* (Open|Cancel|ForceIdentify|RequestGuildMembers|Status|LongLastAck)\b"
```
Expected: the six frozen signatures from Global Constraints, unchanged. Also confirm `internal/manager` still compiles against them (covered by Step 1).

- [ ] **Step 3: Grep for leftover reconnect-reset hacks and stale field access**

Run:
```bash
cd repos/gateway && grep -rn "resetHeartbeat\|s\.wch = make\|s\.prioch = make\|len(s\.wch)+len(s\.prioch)" internal/gatewayws/
```
Expected: **no matches** (all three hacks removed). Then:
```bash
cd repos/gateway && grep -rn "s\.ctx\|s\.cancel\|s\.wsConn\|s\.lastAck\|s\.lastHB\|s\.curState\|s\.wch\|s\.prioch\|s\.authed" internal/gatewayws/*.go | grep -v _test.go
```
Expected: no matches — these are now `conn` fields accessed as `c.X`.

- [ ] **Step 4: Self-review the diff against the design**

Run `git diff master...HEAD -- internal/gatewayws/ internal/manager/` and confirm: (a) no behavior change beyond Task 4's synchronization; (b) every method in the migration map landed on the right receiver; (c) `Open` is a thin wrapper, `run` holds the body; (d) management methods nil-guard `s.cur`.

- [ ] **Step 5: Record the outcome in repo artifacts**

Append to `agent-progress.md` and update `feature_list.json` per the repo AGENTS.md Definition of Done (what changed, verification evidence: build/vet/`-race` output, rollback = revert the refactor commits). Do **not** deploy as part of this plan — shipping goes through the staging-deploy skill separately.

---

## Self-Review (plan vs. intent)

- **Codex review pass (2026-06-23):** all 8 must-fix items folded in — `last` atomic-on-Session (Decision 8), `guilds`/`backfilled` stay on Session to survive RESUME (Decision 9), `run(parent)` preserves the two-context split (Decision 10), `s.ctx`→`context.Background()` in Session persistence/load methods (Step 8), `Open` defer cancels before clearing `cur` (Step 7), RGM-drop & Status/LongLastAck nil-`cur` blessed as documented changes (Decision 11), concurrent `Open` unsupported (Decision 12), and the `resetHeartbeat` test rewritten (Task 3 Step 1) with new nil-`cur` acceptance tests (Step 3b).
- **Coverage:** the captured-channel orphaning (Decision 6, Task 2 Step 7 drops the swap), heartbeat reset loop (Task 2 Step 7 drops `resetHeartbeat`), INVALID_SESSION channel rebuild (Task 2 Step 6), and the `lastAck`/`curState` race (Task 4) are each tied to a concrete step. The frozen external API is verified in Task 5 Step 2.
- **Type consistency:** accessor names (`setState`, `markHB`, `markAck`, `markReady`, `snapshot`), `newConn(ctx, cancel)`, `(*conn).run`, and `s.cur` are used identically across Tasks 2–5.
- **Known imperfection (honest):** Task 2 is a single Go type migration that does not compile until its final step — this is inherent to changing a receiver type across a package, not a placeholder. The steps within it are still individually small and ordered; only the *commit* is deferred to the end of Task 3. Every other task is independently green-gated and committed.
