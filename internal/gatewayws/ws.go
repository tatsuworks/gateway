package gatewayws

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/etcd-io/etcd/clientv3/concurrency"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/czlib"
	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/handler"
	"github.com/tatsuworks/gateway/internal/state"
)

const (
	IdentifyMutexRootName = "/gateway/identify/"
	IdentifyWaitTime      = 10 * time.Second
	IdentifyStabilizeTime = 60 * time.Second
	TimeoutAllowance      = 10 * time.Second
)

var skipMemberRequest = os.Getenv("SKIP_MEMBER_REQUEST") == "true"

// Session is the durable, reused-across-reconnects identity of a shard. It owns
// shared dependencies and the resume tuple, but NOT the live connection: each
// Open builds a fresh conn (see conn.go) and publishes it to cur so management
// calls (Cancel/ForceIdentify/RequestGuildMembers/Status/LongLastAck) reach it.
type Session struct {
	wg *sync.WaitGroup

	name string
	log  slog.Logger

	token      string
	intents    Intents
	shardID    int
	shardCount int

	seq       int64
	sessID    string
	resumeURL string
	last      int64 // event-rate baseline; accessed atomically (logTotalEvents goroutine)

	// forceIdentify is set (atomically) by ForceIdentify from any goroutine to
	// request that the next Open discard the resume tuple and IDENTIFY. It is
	// consumed by the read-loop goroutine in applyForceIdentify; sessID/resumeURL
	// are only ever mutated there, never from the caller of ForceIdentify.
	forceIdentify int32

	lastIdentify time.Time

	rl *rate.Limiter

	// guilds/backfilled are mutated and read only on the read-loop goroutine
	// (READY repopulates them, GUILD_CREATE reads them) and deliberately survive
	// a RESUME, so the backfill-skip optimization keeps working across resumes.
	// They stay on Session for that reason, not on the per-Open conn.
	guilds     map[int64]struct{}
	backfilled map[int64]struct{}

	bufferPool *sync.Pool
	enc        discord.Encoding

	etcd *clientv3.Client

	state   *handler.Client
	stateDB state.DB
	rc      []*redis.Client

	whitelistedEvents map[string]map[string]struct{}

	hasGuildMembersIntent bool

	// cur is the current connection, published by Open and cleared on return.
	// nil before the first Open and briefly between reconnects.
	cur atomic.Pointer[conn]
}

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

func (c *conn) cleanupBuffer() {
	if c.buf != nil {
		if c.buf.Cap() < (1 << 24) {
			c.buf.Reset()
			c.s.bufferPool.Put(c.buf)
		} else {
			c.s.log.Debug(c.ctx, "buffer evicted", slog.F("size", c.buf.Cap()))
		}
	}
	c.buf = nil
}

func (c *conn) GatewayURL() string {
	wsOpts := "?v=10&encoding=" + c.s.enc.Name() + "&compress=zlib-stream"

	if c.s.resumeURL != "" && c.s.sessID != "" {
		return c.s.resumeURL + wsOpts
	}

	return "wss://gateway.discord.gg/" + wsOpts
}

type SessionConfig struct {
	Name              string
	Logger            slog.Logger
	DB                state.DB
	WorkGroup         *sync.WaitGroup
	Redis             []*redis.Client
	Etcd              *clientv3.Client
	Token             string
	Intents           Intents
	ShardID           int
	ShardCount        int
	BufferPool        *sync.Pool
	WhitelistedEvents map[string]map[string]struct{}
}

func NewSession(cfg *SessionConfig) (*Session, error) {
	sess := &Session{
		name:       cfg.Name,
		wg:         cfg.WorkGroup,
		log:        cfg.Logger.With(slog.F("name", cfg.Name), slog.F("shard", cfg.ShardID)),
		token:      cfg.Token,
		shardID:    cfg.ShardID,
		shardCount: cfg.ShardCount,
		intents:    cfg.Intents,

		rl: rate.NewLimiter(1.75, 2),

		etcd: cfg.Etcd,

		state:             handler.NewClient(cfg.Logger, cfg.DB),
		stateDB:           cfg.DB,
		enc:               cfg.DB.Encoding(),
		rc:                cfg.Redis,
		bufferPool:        cfg.BufferPool,
		whitelistedEvents: cfg.WhitelistedEvents,
	}

	// Set hasGuildMembersIntent
	for _, intent := range sess.intents {
		if intent == IntentGuildMembers {
			sess.hasGuildMembersIntent = true
			break
		}
	}

	sess.loadSessID()
	sess.loadSeq()
	sess.loadResumeURL()

	return sess, nil
}

func (s *Session) shouldProcessMembers() bool {
	return !skipMemberRequest && s.hasGuildMembersIntent
}
func (s *Session) calcIdentifyWait() time.Duration {
	totalWaitTime := IdentifyWaitTime
	if s.shouldProcessMembers() { // allow for more time to process database when getting guild members population
		totalWaitTime += IdentifyStabilizeTime
	}
	return totalWaitTime
}

func (c *conn) initEtcd() error {
	timeoutDuration := c.s.calcIdentifyWait() + TimeoutAllowance

	sess, err := concurrency.NewSession(c.s.etcd, concurrency.WithContext(c.ctx), concurrency.WithTTL(int(timeoutDuration.Seconds())))
	if err != nil {
		return xerrors.Errorf("get etcd session: %w", err)
	}

	c.etcdSess = sess
	c.identifyMu = concurrency.NewMutex(sess, IdentifyMutexRootName+strconv.Itoa(c.s.shardID%16))
	return nil
}

func (s *Session) shouldResume() bool {
	return atomic.LoadInt64(&s.seq) != 0 && s.sessID != ""
}

// ForceIdentify requests that the shard discard its resume tuple and IDENTIFY
// on its next connect instead of resuming. It is safe to call from any
// goroutine (e.g. the gRPC management handler): it only sets an atomic flag and
// cancels the active connection. The resume-state mutation itself happens in
// the read-loop goroutine via applyForceIdentify, so sessID/resumeURL are never
// written concurrently. persistShardInfo is flag-aware, so the moment the flag
// is set the cleared tuple is what gets persisted — a crash before reconnect
// cannot leave a resumable row behind. If no connection is active the flag still
// persists and the next Open's applyForceIdentify consumes it.
func (s *Session) ForceIdentify() {
	atomic.StoreInt32(&s.forceIdentify, 1)
	s.Cancel()
}

// applyForceIdentify clears the resume tuple if a force-identify is pending.
// It must run on the read-loop goroutine (it mutates sessID/resumeURL) and is
// called at the top of run before the resume decision.
func (s *Session) applyForceIdentify() {
	if atomic.SwapInt32(&s.forceIdentify, 0) == 0 {
		return
	}
	s.log.Info(context.Background(), "force identify requested, discarding resume state", slog.F("shard", s.shardID))
	atomic.StoreInt64(&s.seq, 0)
	s.sessID = ""
	s.resumeURL = ""
	s.persistShardInfo()
}

func (s *Session) Open(ctx context.Context, token string) error {
	s.wg.Add(1)
	defer s.wg.Done()

	cctx, cancel := context.WithCancel(ctx)

	c := s.newConn(cctx, cancel)
	s.cur.Store(c)
	// Cancel the connection's goroutines BEFORE retracting it from cur, so a
	// concurrent Cancel/ForceIdentify never sees a nil cur while this
	// connection's context is still live.
	defer func() {
		cancel()
		s.cur.Store(nil)
	}()

	// parent ctx is threaded into run so state-handling work survives a
	// disconnect (see run); c.ctx is the connection-scoped context.
	return c.run(ctx)
}

func (c *conn) run(parent context.Context) error {
	defer c.authed.Store(false)

	c.curState = "begin"

	c.s.log.Info(parent, "encoding", slog.F("name", c.s.enc.Name()))

	// Consume any pending force-identify before deciding whether to resume, so a
	// forced shard discards its resume tuple and IDENTIFYs this connect.
	c.s.applyForceIdentify()

	var err error
	err = c.initEtcd()
	if err != nil {
		return err
	}

	// only acquire the identify lock if we know we won't send a resume
	if !c.s.shouldResume() {
		c.s.log.Debug(c.ctx, "acquiring lock, no ability to resume")
		err = c.acquireIdentifyLock()
		if err != nil {
			return xerrors.Errorf("grab identify lock: %w", err)
		}
		c.s.log.Debug(c.ctx, "lock acquired")

	} else {
		c.s.log.Debug(c.ctx, "skipping lock, attempting resume", slog.F("sess", c.s.sessID), slog.F("seq", atomic.LoadInt64(&c.s.seq)))
	}

	r, err := czlib.NewReader(bytes.NewReader(nil))
	if err != nil {
		return xerrors.Errorf("initialize zlib: %w", err)
	}
	c.zr = r
	defer r.Close()

	c.curState = "connecting"
	ws, _, err := websocket.Dial(c.ctx, c.GatewayURL(), nil)
	if err != nil {
		return xerrors.Errorf("dial gateway: %w", err)
	}
	c.wsConn = ws
	c.wsConn.SetReadLimit(512 << 20)

	c.curState = "read hello"
	err = c.readHello()
	if err != nil {
		return xerrors.Errorf("handle hello message: %w", err)
	}

	go c.writer()
	if c.s.shouldResume() {
		c.s.log.Info(c.ctx, "sending resume")
		c.writeResume()
	} else {
		atomic.StoreInt64(&c.s.last, 0)
		c.s.log.Info(c.ctx, "sending identify")
		c.writeIdentify()
		c.s.lastIdentify = time.Now()
	}

	go c.sendHeartbeats()
	go c.logTotalEvents()
	// go c.rotateStatuses()

	c.s.log.Info(c.ctx, "websocket connected, waiting for events")
	defer c.s.persistShardInfo()

	for {
		c.s.log.Debug(c.ctx, "received event", slog.F("last_ack", c.lastAck), slog.F("last_hb", c.lastHB), slog.F("seq", atomic.LoadInt64(&c.s.seq)))

		ev, err := c.readAndDecodeEvent()
		if err != nil {
			c.s.log.Error(c.ctx, "read and decode event", slog.Error(err))
			break
		}
		if ev.S != 0 {
			atomic.StoreInt64(&c.s.seq, ev.S)
		}
		c.s.log.Debug(c.ctx, "decoded event", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))

		c.curState = "handle internal event " + ev.T
		c.s.log.Debug(c.ctx, "handling internal event", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		var handled bool
		if handled, err = c.handleInternalEvent(ev); handled {
			if err != nil {
				break
			}
			continue
		}

		c.curState = "handle state event " + ev.T
		c.s.log.Debug(c.ctx, "handling state event", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		// parent ctx: in-flight state/DB work must NOT be aborted by a disconnect.
		evtPayload, err := c.s.state.HandleEvent(parent, ev)
		if err != nil {
			c.s.log.Error(c.ctx, "handle state event", slog.Error(err))
			continue
		}

		c.curState = "push event to redis"
		c.s.log.Debug(c.ctx, "pushing event to redis", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		c.pushEventToRedis(ev)

		// request guild members on GUILD_CREATE, gated by the backfill marker
		if c.s.shouldProcessMembers() && ev.T == "GUILD_CREATE" && evtPayload != nil && evtPayload.GuildID != 0 {
			c.curState = "maybe request guild members"
			// parent ctx: same rationale as HandleEvent above.
			c.maybeRequestGuildMembers(parent, evtPayload)
		}

	}

	c.curState = "close"
	_ = ws.Close(4000, "")
	c.s.log.Info(c.ctx, "closed")
	return err
}

func (c *conn) pushEventToRedis(ev *discord.Event) {
	if ev.T == "GUILD_MEMBER_CHUNK" {
		return
	}
	push := func(addr string, rc *redis.Client) {
		if whitelist, exists := c.s.whitelistedEvents[addr]; exists {
			c.s.log.Debug(c.ctx, "checking whitelist", slog.F("event_type", ev.T), slog.F("redis_addr", addr))
			if _, ok := whitelist[ev.T]; !ok {
				c.s.log.Debug(c.ctx, "not whitelisted", slog.F("event_type", ev.T), slog.F("redis_addr", addr))
				return
			}
		}

		c.s.log.Debug(c.ctx, "pushing event to redis", slog.F("event_type", ev.T), slog.F("redis_addr", addr))
		if err := rc.RPush(c.ctx, "gateway:events:"+ev.T, ev.D).Err(); err != nil {
			c.s.log.Error(c.ctx, "push event to redis", slog.Error(err), slog.F("event_type", ev.T), slog.F("redis_addr", addr))
		}
	}

	for _, rc := range c.s.rc {
		push(rc.Options().Addr, rc)
	}
}

func (c *conn) handleInternalEvent(ev *discord.Event) (bool, error) {
	switch ev.Op {
	case 1:
		c.writeHeartbeat()
		return true, nil

	// RESUME
	case 6:
		c.s.log.Info(c.ctx, "resumed")
		c.authed.Store(true)
		c.ready = time.Now()

		return true, nil

	// RECONNECT
	case 7:
		c.s.log.Info(c.ctx, "reconnect requested")

		return true, xerrors.New("reconnect")

	// INVALID_SESSION
	case 9:
		c.s.log.Info(c.ctx, "invalid session, reconnecting")
		c.s.sessID = ""
		atomic.StoreInt64(&c.s.seq, 0)
		c.s.resumeURL = ""
		c.s.persistShardInfo()

		if c.identifyMu.IsOwner().Result == etcdserverpb.Compare_EQUAL {
			err := c.releaseIdentifyLock()
			if err != nil {
				c.s.log.Error(c.ctx, "release held identify lock after invalid session", slog.Error(err))
			}
		}

		return true, xerrors.New("invalid session")

	// HEARTBEAT_ACK
	case 11:
		c.lastAck = time.Now()
		return true, nil
	}

	switch ev.T {
	case "READY":
		c.s.guilds = map[int64]struct{}{}
		guilds, _, sess, resumeURL, err := c.s.enc.DecodeReady(ev.D)
		if err != nil {
			return true, xerrors.Errorf("decode ready: %w", err)
		}

		for i := range guilds {
			c.s.guilds[i] = struct{}{}
		}

		// No-expiry skip (reconnect-storm speed is the priority): skip RGM for any
		// guild that has ever *completed* a backfill, regardless of age, so a mass
		// re-identify after long uptime skips every populated guild instead of
		// re-backfilling all of them. GetGuildBackfillTimes already filters to
		// backfilled_at IS NOT NULL, and the completion marker is only stamped on
		// the final chunk (or a small-guild payload reconcile) — so unlike a bare
		// EXISTS(members) probe this never skips a partial/interrupted roster.
		// Roster drift is accepted as small: live member events keep rosters
		// accurate whenever connected, so drift only accrues during rare/short
		// disconnect windows, and the UNLOGGED cache is reset wholesale on any
		// unclean PG restart (forcing a cold re-backfill). An optional Phase 3b
		// background sweep can bound it further if staleness ever becomes a real
		// problem; it is not required for this skip to be correct.
		c.s.backfilled = map[int64]struct{}{}
		ids := make([]int64, 0, len(c.s.guilds))
		for id := range c.s.guilds {
			ids = append(ids, id)
		}
		if times, terr := c.s.stateDB.GetGuildBackfillTimes(c.ctx, ids); terr != nil {
			c.s.log.Error(c.ctx, "preload guild backfill times", slog.Error(terr))
		} else {
			for id := range times {
				c.s.backfilled[id] = struct{}{}
			}
		}

		c.s.sessID = sess
		c.s.resumeURL = resumeURL
		c.s.log.Info(c.ctx, "ready", slog.F("sess", sess), slog.F("resume_gateway_url", resumeURL),
			slog.F("guild_count", len(c.s.guilds)))
		c.s.persistShardInfo()
		c.authed.Store(true)
		c.ready = time.Now()

		go func() {
			totalWaitTime := c.s.calcIdentifyWait()
			time.Sleep(totalWaitTime)
			if err := c.releaseIdentifyLock(); err != nil {
				c.s.log.Error(c.ctx, "release identify lock after ready", slog.Error(err))
			}
		}()

		return true, nil

	case "RESUMED":
		c.s.log.Info(c.ctx, "resumed")
		c.authed.Store(true)
		c.ready = time.Now()

		return true, nil
	}

	return false, nil
}

func (c *conn) acquireIdentifyLock() error {
	timeoutLock, cancel := context.WithTimeout(c.ctx, time.Second*160)
	defer cancel()

	err := c.identifyMu.Lock(timeoutLock)
	if err != nil {
		return xerrors.Errorf("acquire identify lock: %w", err)
	}

	return nil
}

func (c *conn) releaseIdentifyLock() error {
	c.s.log.Info(c.ctx, "release identify lock", slog.F("key", c.identifyMu.Key()))
	if c.identifyMu.Key() != "" {
		err := c.identifyMu.Unlock(c.ctx)
		if err != nil {
			return xerrors.Errorf("release identify lock: %w", err)
		}
	}
	return nil
}

func (s *Session) Cancel() {
	if c := s.cur.Load(); c != nil {
		c.cancel()
	}
}

func (s *Session) RequestGuildMembers(guildID int64) {
	c := s.cur.Load()
	if c == nil {
		s.log.Info(context.Background(), "drop members request: no active connection", slog.F("guild", guildID))
		return
	}
	c.requestGuildMembersExternal(guildID)
}

func (c *conn) readAndDecodeEvent() (*discord.Event, error) {
	c.curState = "read message"
	c.buf = c.s.bufferPool.Get().(*bytes.Buffer)
	defer c.cleanupBuffer()

	err := c.readMessage()
	if err != nil {
		var werr websocket.CloseError
		if xerrors.As(err, &werr) {
			// This somehow happens if you resume to a
			// valid session associated with a different
			// token.
			if werr.Code == 4006 {
				atomic.StoreInt64(&c.s.seq, 0)
				c.s.sessID = ""
				c.s.resumeURL = ""
				c.s.persistShardInfo()
			}
		}
		return nil, xerrors.Errorf("read message: %w", err)
	}

	c.curState = "decode event"
	var ev *discord.Event
	ev, err = c.s.enc.DecodeT(c.buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("decode event: %w", err)
	}

	return ev, nil
}
