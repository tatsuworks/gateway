package gatewayws

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

	// readyGrace bounds how long we wait for READY before unblocking the
	// identify bucket anyway. Discord's identify pacing only cares about
	// how long ago the IDENTIFY op was sent, not whether READY arrived,
	// so once IdentifyWaitTime + readyGrace has elapsed it is safe to
	// release the bucket regardless.
	readyGrace = 5 * time.Second
)

var skipMemberRequest = os.Getenv("SKIP_MEMBER_REQUEST") == "true"

type Session struct {
	ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup

	name string
	log  slog.Logger

	token      string
	intents    Intents
	shardID    int
	shardCount int

	authed    bool
	seq       int64
	sessID    string
	resumeURL string
	last      int64

	wsConn *websocket.Conn
	zr     io.ReadCloser

	interval time.Duration
	trace    string

	rl     *rate.Limiter
	wch    chan *Op
	prioch chan *Op

	lastHB       time.Time
	lastAck      time.Time
	ready        time.Time
	lastIdentify time.Time

	guilds   map[int64]struct{}
	curState string

	bufferPool *sync.Pool
	buf        *bytes.Buffer
	enc        discord.Encoding

	etcd       *clientv3.Client
	etcdSess   *concurrency.Session
	identifyMu *concurrency.Mutex

	state   *handler.Client
	stateDB state.DB
	rc      []*redis.Client

	whitelistedEvents map[string]map[string]struct{}

	hasGuildMembersIntent bool

	// stabilizeSem caps the number of shards that can be in the
	// post-IDENTIFY backfill window concurrently. Acquired after READY
	// when this shard intends to request guild members; held for
	// stabilizeDuration (or until ctx cancels). Decoupled from the
	// identify mutex so it does not pace identifies.
	stabilizeSem         chan struct{}
	stabilizeDuration    time.Duration
	stabilizeMaxDuration time.Duration
	stabilizeUseDrain    bool
	identifyPacing       time.Duration

	// divergenceRatio is the fractional gap between Discord's reported
	// member_count and the cached count above which a Request Guild
	// Members op is sent on GUILD_CREATE. 0 disables the check (always
	// request, current pre-Phase-3 behavior). Cold guilds always
	// request regardless.
	divergenceRatio  float64
	divergenceTiming *DivergenceTiming

	chunks *chunkTracker
}

func (s *Session) Status() string {
	return fmt.Sprintf("%v: %s [LastAck: %v]", s.shardID, s.curState, s.lastAck.Format(time.RFC3339))
}

func (s *Session) LongLastAck(threshold time.Duration) bool {
	cutoff := time.Now().Add(-threshold)
	return s.lastAck.Before(cutoff) && s.ready.Before(cutoff)
}

func (s *Session) cleanupBuffer() {
	if s.buf != nil {
		if s.buf.Cap() < (1 << 24) {
			s.buf.Reset()
			s.bufferPool.Put(s.buf)
		} else {
			s.log.Debug(s.ctx, "buffer evicted", slog.F("size", s.buf.Cap()))
		}
	}
	s.buf = nil
}

func (s *Session) GatewayURL() string {
	wsOpts := "?v=10&encoding=" + s.enc.Name() + "&compress=zlib-stream"

	if s.resumeURL != "" && s.sessID != "" {
		return s.resumeURL + wsOpts
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

	// StabilizeSem is a buffered channel acting as a weighted semaphore
	// across all shards in this manager process. Capacity controls how
	// many shards may be in the post-IDENTIFY backfill window at once.
	// nil disables the gate (no waiting).
	StabilizeSem chan struct{}
	// StabilizeDuration is the per-shard hold time for the stabilize
	// semaphore. Zero disables the wait.
	StabilizeDuration time.Duration
	// StabilizeMaxDuration is an upper bound enforced via context, used
	// if anything (e.g. shard cancellation) goes wrong while holding
	// the semaphore. Defaults to 2x StabilizeDuration if zero.
	StabilizeMaxDuration time.Duration
	// StabilizeUseDrain switches holdStabilize from a fixed-duration
	// sleep (Phase 1) to a per-shard chunk-drain wait (Phase 2). The
	// StabilizeDuration becomes a soft floor; StabilizeMaxDuration is
	// the hard cap.
	StabilizeUseDrain bool
	// IdentifyPacing is how long the identify mutex is held after
	// IDENTIFY is sent. Zero falls back to IdentifyWaitTime.
	IdentifyPacing time.Duration
	// DivergenceRatio gates GUILD_CREATE-time member backfill. See
	// Session.divergenceRatio.
	DivergenceRatio float64
	// DivergenceTiming, when non-nil, records per-call durations for
	// the divergence DB read. Shared across shards in a process.
	DivergenceTiming *DivergenceTiming
}

func NewSession(cfg *SessionConfig) (*Session, error) {
	pacing := cfg.IdentifyPacing
	if pacing <= 0 {
		pacing = IdentifyWaitTime
	}
	stabilizeMax := cfg.StabilizeMaxDuration
	if stabilizeMax <= 0 && cfg.StabilizeDuration > 0 {
		stabilizeMax = 2 * cfg.StabilizeDuration
	}

	sess := &Session{
		ctx:        context.Background(),
		name:       cfg.Name,
		wg:         cfg.WorkGroup,
		log:        cfg.Logger.With(slog.F("name", cfg.Name), slog.F("shard", cfg.ShardID)),
		token:      cfg.Token,
		shardID:    cfg.ShardID,
		shardCount: cfg.ShardCount,
		intents:    cfg.Intents,

		// start with a 1kb buffer
		rl:     rate.NewLimiter(1.75, 2),
		wch:    make(chan *Op, 2000),
		prioch: make(chan *Op),

		etcd: cfg.Etcd,

		state:             handler.NewClient(cfg.Logger, cfg.DB),
		stateDB:           cfg.DB,
		enc:               cfg.DB.Encoding(),
		rc:                cfg.Redis,
		bufferPool:        cfg.BufferPool,
		whitelistedEvents: cfg.WhitelistedEvents,

		stabilizeSem:         cfg.StabilizeSem,
		stabilizeDuration:    cfg.StabilizeDuration,
		stabilizeMaxDuration: stabilizeMax,
		stabilizeUseDrain:    cfg.StabilizeUseDrain,
		identifyPacing:       pacing,
		divergenceRatio:      cfg.DivergenceRatio,
		divergenceTiming:     cfg.DivergenceTiming,

		chunks: newChunkTracker(),
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

// shouldRequestMembersForGuild decides whether to send a Request Guild
// Members op for a GUILD_CREATE we just processed.
//
//   - divergenceRatio <= 0 (default): always request, current behavior.
//   - divergenceRatio > 0: lazily look up cached member count for the
//     guild. Cold guilds (cached == 0) always request. When Discord
//     supplied a member_count and the divergence between cached and
//     Discord-reported is within the ratio, skip the request.
//   - When divergence is enabled but Discord did not supply a count,
//     fall through to "request" — same as today.
//
// The DB read happens only when divergence is on, so the slow
// COUNT(*) per GUILD_CREATE that would otherwise hit Postgres during
// mass reconnect is gated behind explicit opt-in.
func (s *Session) shouldRequestMembersForGuild(ctx context.Context, p *handler.EventPayload) bool {
	if p == nil {
		return false
	}
	if s.divergenceRatio <= 0 {
		return true
	}
	if p.DiscordMemberCount <= 0 {
		return true
	}
	start := time.Now()
	cached, err := s.stateDB.GetGuildMemberCount(ctx, p.GuildID)
	s.divergenceTiming.record(ctx, time.Since(start))
	if err != nil {
		s.log.Warn(s.ctx, "divergence check: GetGuildMemberCount failed, falling through to request",
			slog.F("guild", p.GuildID), slog.Error(err))
		return true
	}
	if cached == 0 {
		return true
	}
	diff := p.DiscordMemberCount - int64(cached)
	if diff < 0 {
		diff = -diff
	}
	ratio := float64(diff) / float64(p.DiscordMemberCount)
	if ratio <= s.divergenceRatio {
		s.log.Debug(s.ctx, "skipping guild member backfill, divergence below threshold",
			slog.F("guild", p.GuildID),
			slog.F("discord_count", p.DiscordMemberCount),
			slog.F("cached_count", cached),
			slog.F("ratio", ratio))
		return false
	}
	return true
}

// identifyLockHoldTime is the upper bound on how long this shard intends
// to hold the identify mutex. Sets the etcd lease TTL. The actual hold
// time is identifyPacing + readyGrace; the TTL has additional headroom
// so a brief GC pause or etcd hiccup does not collapse the lease while
// we are still legitimately holding the lock.
func (s *Session) identifyLockHoldTime() time.Duration {
	return s.identifyPacing + readyGrace + TimeoutAllowance
}

func (s *Session) initEtcd() error {
	timeoutDuration := s.identifyLockHoldTime()

	sess, err := concurrency.NewSession(s.etcd, concurrency.WithContext(s.ctx), concurrency.WithTTL(int(timeoutDuration.Seconds())))
	if err != nil {
		return xerrors.Errorf("get etcd session: %w", err)
	}

	s.etcdSess = sess
	s.identifyMu = concurrency.NewMutex(sess, IdentifyMutexRootName+strconv.Itoa(s.shardID%16))
	return nil
}

func (s *Session) shouldResume() bool {
	return s.seq != 0 && s.sessID != ""
}

func (s *Session) Open(ctx context.Context, token string) error {
	s.wg.Add(1)
	defer s.wg.Done()

	defer func() {
		s.authed = false
	}()

	s.curState = "begin"
	s.ctx, s.cancel = context.WithCancel(ctx)
	defer s.cancel()

	s.log.Info(ctx, "encoding", slog.F("name", s.enc.Name()))

	s.lastAck = time.Time{}

	// Fresh tracker per Open(). The tracker is per session-attempt: any
	// RGM whose chunks never arrive (deleted guild, dropped frames,
	// session closed mid-burst) would otherwise leave a permanent
	// entry that prevents drained() from ever firing on the next
	// reconnect, silently regressing the stabilize wait to the
	// max-duration cap.
	s.chunks = newChunkTracker()

	var err error
	err = s.initEtcd()
	if err != nil {
		return err
	}

	r, err := czlib.NewReader(bytes.NewReader(nil))
	if err != nil {
		return xerrors.Errorf("initialize zlib: %w", err)
	}
	s.zr = r
	defer r.Close()

	// Dial + HELLO happen outside the identify mutex. Discord's identify
	// pacing only applies to the IDENTIFY op itself, so doing the WS
	// handshake before holding the lock removes dial/HELLO latency from
	// the bucket's serialized critical section.
	s.curState = "connecting"
	c, _, err := websocket.Dial(s.ctx, s.GatewayURL(), nil)
	if err != nil {
		return xerrors.Errorf("dial gateway: %w", err)
	}
	s.wsConn = c
	s.wsConn.SetReadLimit(512 << 20)

	s.curState = "read hello"
	err = s.readHello()
	if err != nil {
		return xerrors.Errorf("handle hello message: %w", err)
	}

	// only acquire the identify lock if we know we won't send a resume
	if !s.shouldResume() {
		s.log.Debug(s.ctx, "acquiring lock, no ability to resume")
		lockWaitStart := time.Now()
		err = s.acquireIdentifyLock()
		if err != nil {
			return xerrors.Errorf("grab identify lock: %w", err)
		}
		s.log.Info(s.ctx, "identify lock acquired",
			slog.F("wait", time.Since(lockWaitStart).String()))
	} else {
		s.log.Debug(s.ctx, "skipping lock, attempting resume", slog.F("sess", s.sessID), slog.F("seq", s.seq))
	}

	go s.writer()
	if s.shouldResume() {
		s.log.Info(s.ctx, "sending resume")
		s.writeResume()
	} else {
		s.last = 0
		s.log.Info(s.ctx, "sending identify")
		s.writeIdentify()
		if len(s.wch)+len(s.prioch) > 0 {
			s.wch = make(chan *Op, 2000)
			s.prioch = make(chan *Op)
		}
		s.lastIdentify = time.Now()
		// Release the identify mutex after the pacing window elapses,
		// regardless of whether READY has arrived. Discord's identify
		// rate limit is keyed on time since IDENTIFY was sent, not on
		// READY arrival, so other shards in the bucket can identify
		// once the pacing window has elapsed. The READY-handler path
		// is now responsible only for stabilize (backfill draining).
		s.scheduleIdentifyLockRelease()
	}

	go s.sendHeartbeats()
	go s.logTotalEvents()
	// go s.rotateStatuses()

	s.log.Info(s.ctx, "websocket connected, waiting for events")
	defer s.persistShardInfo()

	for {
		s.log.Debug(s.ctx, "received event", slog.F("last_ack", s.lastAck), slog.F("last_hb", s.lastHB), slog.F("seq", atomic.LoadInt64(&s.seq)))

		ev, err := s.readAndDecodeEvent()
		if err != nil {
			s.log.Error(ctx, "read and decode event", slog.Error(err))
			break
		}
		if ev.S != 0 {
			atomic.StoreInt64(&s.seq, ev.S)
		}
		s.log.Debug(s.ctx, "decoded event", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))

		s.curState = "handle internal event " + ev.T
		s.log.Debug(s.ctx, "handling internal event", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		var handled bool
		if handled, err = s.handleInternalEvent(ev); handled {
			if err != nil {
				break
			}
			continue
		}

		s.curState = "handle state event " + ev.T
		s.log.Debug(s.ctx, "handling state event", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		evtPayload, err := s.state.HandleEvent(ctx, ev)
		if err != nil {
			s.log.Error(s.ctx, "handle state event", slog.Error(err))
			continue
		}

		if ev.T == "GUILD_MEMBERS_CHUNK" && evtPayload != nil && s.chunks != nil {
			s.chunks.recordChunk(evtPayload.GuildID, evtPayload.MemberChunkIndex, evtPayload.MemberChunkCount)
		}

		s.curState = "push event to redis"
		s.log.Debug(s.ctx, "pushing event to redis", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		s.pushEventToRedis(ev)

		// Request members on GUILD_CREATE. When divergence checking is
		// disabled (default) this is unconditional — same behavior as
		// pre-Phase-3. When enabled, lazily fetch the cached count and
		// skip the request if cached and Discord-reported counts agree
		// within divergenceRatio. The lazy fetch keeps a slow
		// COUNT(*) off the hot path unless ops opts in.
		if s.shouldProcessMembers() && ev.T == "GUILD_CREATE" && evtPayload != nil && evtPayload.GuildID != 0 {
			if s.shouldRequestMembersForGuild(ctx, evtPayload) {
				s.curState = "request guild members"
				s.log.Debug(s.ctx, "requesting guild members",
					slog.F("guild", evtPayload.GuildID),
					slog.F("discord_count", evtPayload.DiscordMemberCount))
				s.requestGuildMembers(evtPayload.GuildID)
			}
		}

	}

	s.curState = "close"
	_ = c.Close(4000, "")
	s.log.Info(s.ctx, "closed")
	return err
}

func (s *Session) pushEventToRedis(ev *discord.Event) {
	if ev.T == "GUILD_MEMBER_CHUNK" {
		return
	}
	push := func(addr string, rc *redis.Client) {
		if whitelist, exists := s.whitelistedEvents[addr]; exists {
			s.log.Debug(s.ctx, "checking whitelist", slog.F("event_type", ev.T), slog.F("redis_addr", addr))
			if _, ok := whitelist[ev.T]; !ok {
				s.log.Debug(s.ctx, "not whitelisted", slog.F("event_type", ev.T), slog.F("redis_addr", addr))
				return
			}
		}

		s.log.Debug(s.ctx, "pushing event to redis", slog.F("event_type", ev.T), slog.F("redis_addr", addr))
		if err := rc.RPush(s.ctx, "gateway:events:"+ev.T, ev.D).Err(); err != nil {
			s.log.Error(s.ctx, "push event to redis", slog.Error(err), slog.F("event_type", ev.T), slog.F("redis_addr", addr))
		}
	}

	for _, rc := range s.rc {
		push(rc.Options().Addr, rc)
	}
}

func (s *Session) handleInternalEvent(ev *discord.Event) (bool, error) {
	switch ev.Op {
	case 1:
		s.writeHeartbeat()
		return true, nil

	// RESUME
	case 6:
		s.log.Info(s.ctx, "resumed")
		s.authed = true
		s.ready = time.Now()

		return true, nil

	// RECONNECT
	case 7:
		s.log.Info(s.ctx, "reconnect requested")

		return true, xerrors.New("reconnect")

	// INVALID_SESSION
	case 9:
		s.log.Info(s.ctx, "invalid session, reconnecting")
		s.sessID = ""
		s.seq = 0
		s.resumeURL = ""
		s.persistShardInfo()
		s.wch = make(chan *Op, 2000)

		if s.identifyMu.IsOwner().Result == etcdserverpb.Compare_EQUAL {
			err := s.releaseIdentifyLock()
			if err != nil {
				s.log.Error(s.ctx, "release held identify lock after invalid session", slog.Error(err))
			}
		}

		return true, xerrors.New("invalid session")

	// HEARTBEAT_ACK
	case 11:
		s.lastAck = time.Now()
		return true, nil
	}

	switch ev.T {
	case "READY":
		s.guilds = map[int64]struct{}{}
		guilds, _, sess, resumeURL, err := s.enc.DecodeReady(ev.D)
		if err != nil {
			return true, xerrors.Errorf("decode ready: %w", err)
		}

		for i := range guilds {
			s.guilds[i] = struct{}{}
		}

		s.sessID = sess
		s.resumeURL = resumeURL
		s.log.Info(s.ctx, "ready", slog.F("sess", sess), slog.F("resume_gateway_url", resumeURL),
			slog.F("guild_count", len(s.guilds)))
		s.persistShardInfo()
		s.authed = true
		s.ready = time.Now()

		// Identify lock release is scheduled separately right after
		// IDENTIFY is sent (see scheduleIdentifyLockRelease). Here we
		// only need to take the stabilize gate, which paces the
		// per-shard backfill burst (Request Guild Members → chunks)
		// against state.DB writes. Skipped for shards that won't be
		// requesting members.
		if s.shouldProcessMembers() {
			go s.holdStabilize()
		}

		return true, nil

	case "RESUMED":
		s.log.Info(s.ctx, "resumed")
		s.authed = true
		s.ready = time.Now()

		return true, nil
	}

	return false, nil
}

func (s *Session) acquireIdentifyLock() error {
	timeoutLock, cancel := context.WithTimeout(s.ctx, time.Second*160)
	defer cancel()

	err := s.identifyMu.Lock(timeoutLock)
	if err != nil {
		return xerrors.Errorf("acquire identify lock: %w", err)
	}

	return nil
}

func (s *Session) releaseIdentifyLock() error {
	s.log.Info(s.ctx, "release identify lock", slog.F("key", s.identifyMu.Key()))
	if s.identifyMu.Key() != "" {
		err := s.identifyMu.Unlock(s.ctx)
		if err != nil {
			return xerrors.Errorf("release identify lock: %w", err)
		}
	}
	return nil
}

// scheduleIdentifyLockRelease unblocks the identify bucket after the
// pacing window elapses. Runs in a goroutine; survives the parent's
// event loop unwinding via context.Background, but races against
// invalid-session cleanup which releases the lock eagerly. The
// IsOwner guard makes the second release a no-op.
func (s *Session) scheduleIdentifyLockRelease() {
	pacing := s.identifyPacing
	if pacing <= 0 {
		pacing = IdentifyWaitTime
	}
	hold := pacing + readyGrace
	go func() {
		t := time.NewTimer(hold)
		defer t.Stop()
		select {
		case <-t.C:
		case <-s.ctx.Done():
		}
		if s.identifyMu == nil || s.identifyMu.Key() == "" {
			return
		}
		if s.identifyMu.IsOwner().Result != etcdserverpb.Compare_EQUAL {
			return
		}
		if err := s.releaseIdentifyLock(); err != nil {
			s.log.Error(s.ctx, "release identify lock after pacing", slog.Error(err))
		}
	}()
}

// holdStabilize acquires the cross-shard stabilize semaphore (if
// configured) and holds it to pace per-shard backfill bursts. With
// stabilizeUseDrain=true (Phase 2), the hold lasts until the shard's
// chunk-drain tracker reports zero pending, with stabilizeDuration as
// a floor and stabilizeMaxDuration as a cap. With stabilizeUseDrain=
// false (Phase 1 rollback), the hold is a fixed stabilizeDuration.
// Runs in its own goroutine — does not block the caller.
func (s *Session) holdStabilize() {
	if s.stabilizeSem == nil || s.stabilizeDuration <= 0 {
		return
	}

	capDur := s.stabilizeMaxDuration
	if capDur <= 0 {
		capDur = 2 * s.stabilizeDuration
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
		hold := min(s.stabilizeDuration, capDur)
		t := time.NewTimer(hold)
		defer t.Stop()
		select {
		case <-t.C:
		case <-s.ctx.Done():
		}
		return
	}

	floor := time.NewTimer(s.stabilizeDuration)
	defer floor.Stop()
	capT := time.NewTimer(capDur)
	defer capT.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-capT.C:
			s.log.Warn(s.ctx, "stabilize hit max-duration cap before drain",
				slog.F("pending", s.chunks.pendingCount()))
			return
		case <-s.chunks.drained():
			select {
			case <-floor.C:
				return
			case <-s.ctx.Done():
				return
			case <-capT.C:
				return
			}
		}
	}
}

func (s *Session) Cancel() {
	s.cancel()
}

func (s *Session) readAndDecodeEvent() (*discord.Event, error) {
	s.curState = "read message"
	s.buf = s.bufferPool.Get().(*bytes.Buffer)
	defer s.cleanupBuffer()

	err := s.readMessage()
	if err != nil {
		var werr websocket.CloseError
		if xerrors.As(err, &werr) {
			// This somehow happens if you resume to a
			// valid session associated with a different
			// token.
			if werr.Code == 4006 {
				s.seq = 0
				s.sessID = ""
				s.resumeURL = ""
				s.persistShardInfo()
			}
		}
		return nil, xerrors.Errorf("read message: %w", err)
	}

	s.curState = "decode event"
	var ev *discord.Event
	ev, err = s.enc.DecodeT(s.buf.Bytes())
	if err != nil {
		return nil, xerrors.Errorf("decode event: %w", err)
	}

	return ev, nil
}
