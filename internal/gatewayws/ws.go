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

	guilds     map[int64]struct{}
	backfilled map[int64]struct{}
	curState   string

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
}

func NewSession(cfg *SessionConfig) (*Session, error) {
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

func (s *Session) initEtcd() error {
	timeoutDuration := s.calcIdentifyWait() + TimeoutAllowance

	sess, err := concurrency.NewSession(s.etcd, concurrency.WithContext(s.ctx), concurrency.WithTTL(int(timeoutDuration.Seconds())))
	if err != nil {
		return xerrors.Errorf("get etcd session: %w", err)
	}

	s.etcdSess = sess
	s.identifyMu = concurrency.NewMutex(sess, IdentifyMutexRootName+strconv.Itoa(s.shardID%16))
	return nil
}

func (s *Session) shouldResume() bool {
	return atomic.LoadInt64(&s.seq) != 0 && s.sessID != ""
}

func (s *Session) ForceIdentify() {
	atomic.StoreInt64(&s.seq, 0)
	s.sessID = ""
	s.resumeURL = ""
	s.persistShardInfo()
	s.Cancel()
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

	var err error
	err = s.initEtcd()
	if err != nil {
		return err
	}

	// only acquire the identify lock if we know we won't send a resume
	if !s.shouldResume() {
		s.log.Debug(s.ctx, "acquiring lock, no ability to resume")
		err = s.acquireIdentifyLock()
		if err != nil {
			return xerrors.Errorf("grab identify lock: %w", err)
		}
		s.log.Debug(s.ctx, "lock acquired")

	} else {
		s.log.Debug(s.ctx, "skipping lock, attempting resume", slog.F("sess", s.sessID), slog.F("seq", s.seq))
	}

	r, err := czlib.NewReader(bytes.NewReader(nil))
	if err != nil {
		return xerrors.Errorf("initialize zlib: %w", err)
	}
	s.zr = r
	defer r.Close()

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

		s.curState = "push event to redis"
		s.log.Debug(s.ctx, "pushing event to redis", slog.F("op", ev.Op), slog.F("type", ev.T), slog.F("seq", ev.S))
		s.pushEventToRedis(ev)

		// request guild members on GUILD_CREATE, gated by the backfill marker
		if s.shouldProcessMembers() && ev.T == "GUILD_CREATE" && evtPayload != nil && evtPayload.GuildID != 0 {
			s.curState = "maybe request guild members"
			s.maybeRequestGuildMembers(ctx, evtPayload)
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
		s.backfilled = map[int64]struct{}{}
		ids := make([]int64, 0, len(s.guilds))
		for id := range s.guilds {
			ids = append(ids, id)
		}
		if times, terr := s.stateDB.GetGuildBackfillTimes(s.ctx, ids); terr != nil {
			s.log.Error(s.ctx, "preload guild backfill times", slog.Error(terr))
		} else {
			for id := range times {
				s.backfilled[id] = struct{}{}
			}
		}

		s.sessID = sess
		s.resumeURL = resumeURL
		s.log.Info(s.ctx, "ready", slog.F("sess", sess), slog.F("resume_gateway_url", resumeURL),
			slog.F("guild_count", len(s.guilds)))
		s.persistShardInfo()
		s.authed = true
		s.ready = time.Now()

		go func() {
			totalWaitTime := s.calcIdentifyWait()
			time.Sleep(totalWaitTime)
			err = s.releaseIdentifyLock()
			if err != nil {
				s.log.Error(s.ctx, "release identify lock after ready", slog.Error(err))
			}
		}()

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

func (s *Session) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
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
