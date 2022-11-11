package gatewayws

import (
	"bytes"
	"context"
	"fmt"
	"github.com/tatsuworks/gateway/queuepb"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/etcd-io/etcd/clientv3/concurrency"
	"github.com/go-redis/redis"
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
)

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

	authed bool
	seq    int64
	sessID string
	last   int64

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
	rc      *redis.Client
	queuec  queuepb.QueueClient

	whitelistedEvents map[string]struct{}
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
	return "wss://gateway.discord.gg/?v=10&encoding=" + s.enc.Name() + "&compress=zlib-stream"
}

type SessionConfig struct {
	Name              string
	Logger            slog.Logger
	DB                state.DB
	WorkGroup         *sync.WaitGroup
	Redis             *redis.Client
	Etcd              *clientv3.Client
	Queue             queuepb.QueueClient
	Token             string
	Intents           Intents
	ShardID           int
	ShardCount        int
	BufferPool        *sync.Pool
	WhitelistedEvents map[string]struct{}
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
		queuec:            cfg.Queue,
		bufferPool:        cfg.BufferPool,
		whitelistedEvents: cfg.WhitelistedEvents,
	}

	sess.loadSessID()
	sess.loadSeq()
	return sess, nil
}

func (s *Session) initEtcd() error {
	sess, err := concurrency.NewSession(s.etcd, concurrency.WithContext(s.ctx), concurrency.WithTTL(20))
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
	defer s.persistSeq()

	for {
		ev, err := s.readAndDecodeEvent()
		if err != nil {
			s.log.Error(ctx, "read and decode event", slog.Error(err))
			break
		}
		if ev.S != 0 {
			atomic.StoreInt64(&s.seq, ev.S)
		}

		s.curState = "handle internal event " + ev.T
		var handled bool
		if handled, err = s.handleInternalEvent(ev); handled {
			if err != nil {
				break
			}

			continue
		}

		s.curState = "handle state event " + ev.T
		evtPayload, err := s.state.HandleEvent(ctx, ev)
		if err != nil {
			s.log.Error(s.ctx, "handle state event", slog.Error(err))
			continue
		}

		s.curState = "push event to redis"
		s.pushEventToRedis(ev, evtPayload)

		s.curState = "push event to queue"
		s.pushEventToQueue(ev, evtPayload)

		s.curState = "request guild members"
		// only request members from new guilds.
		// if _, ok := s.guilds[requestMembers]; requestMembers != 0 && !ok {
		shouldDoGuildMemberRequest := s.lastIdentify.IsZero() || s.lastIdentify.Add(5*time.Minute).Before(time.Now())
		if evtPayload != nil && evtPayload.GuildID != 0 && shouldDoGuildMemberRequest {
			s.log.Debug(s.ctx, "requesting guild members", slog.F("guild", evtPayload.GuildID))
			s.requestGuildMembers(evtPayload.GuildID)
		}
	}

	s.curState = "close"
	_ = c.Close(4000, "")
	s.log.Info(s.ctx, "closed")
	return err
}

func (s *Session) pushEventToRedis(ev *discord.Event, evtPayload *handler.EventPayload) {
	if s.whitelistedEvents != nil {
		if _, ok := s.whitelistedEvents[ev.T]; !ok {
			s.log.Debug(s.ctx, "not whitelisted", slog.F("event type", ev.T))
			return
		}
	}
	if (ev.T != "GUILD_CREATE" || evtPayload.IsNewGuild) &&
		ev.T != "GUILD_MEMBER_CHUNK" {
		err := s.rc.RPush("gateway:events:"+ev.T, ev.D).Err()
		if err != nil {
			s.log.Error(s.ctx, "push event to redis", slog.Error(err))
		}
	}
}

func (s *Session) pushEventToQueue(ev *discord.Event, evtPayload *handler.EventPayload) {
	if s.whitelistedEvents != nil {
		if _, ok := s.whitelistedEvents[ev.T]; !ok {
			s.log.Debug(s.ctx, "not whitelisted", slog.F("event type", ev.T))
			return
		}
	}

	if (ev.T != "GUILD_CREATE" || evtPayload.IsNewGuild) &&
		ev.T != "GUILD_MEMBER_CHUNK" {
		_, err := s.queuec.Listener(s.ctx, &queuepb.EventRequest{Key: "events:" + ev.T, Value: ev.D})
		if err != nil {
			s.log.Error(s.ctx, "push event to queue", slog.Error(err))
		}
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
		s.persistSessID()
		s.seq = 0
		s.persistSeq()
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
		guilds, _, sess, err := s.enc.DecodeReady(ev.D)
		if err != nil {
			return true, xerrors.Errorf("decode ready: %w", err)
		}

		for i := range guilds {
			s.guilds[i] = struct{}{}
		}

		s.sessID = sess
		s.log.Info(s.ctx, "ready", slog.F("sess", sess), slog.F("guild_count", len(s.guilds)))
		s.persistSessID()
		s.authed = true
		s.ready = time.Now()

		go func() {
			time.Sleep(7 * time.Second)
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
				s.persistSeq()
				s.persistSessID()
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
