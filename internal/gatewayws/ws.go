package gatewayws

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/etcd-io/etcd/clientv3/concurrency"
	"github.com/go-redis/redis"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/czlib"
	"github.com/tatsuworks/gateway/discordetf"
	"github.com/tatsuworks/gateway/handler"
)

const IdentifyMutexName = "/gateway/identify"

var (
	GatewayETF = "wss://gateway.discord.gg/?v=6&encoding=etf&compress=zlib-stream"
)

type Session struct {
	ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup

	log *zap.Logger

	token   string
	shardID int
	shards  int

	seq    int64
	sessID string
	last   int64

	wsConn *websocket.Conn
	zr     io.ReadCloser

	interval time.Duration
	trace    string

	lastHB  time.Time
	lastAck time.Time

	buf  *bytes.Buffer
	hbuf *bytes.Buffer

	etcd       *clientv3.Client
	etcdSess   *concurrency.Session
	identifyMu *concurrency.Mutex

	state *handler.Client
	rc    *redis.Client
}

func NewSession(
	logger *zap.Logger,
	wg *sync.WaitGroup,
	rdb *redis.Client,
	etcdCli *clientv3.Client,
	token string,
	shardID, shards int,
) (*Session, error) {
	c, err := handler.NewClient()
	if err != nil {
		return nil, xerrors.Errorf("failed to create state handler: %w", err)
	}

	sess := &Session{
		wg:      wg,
		log:     logger.With(zap.Int("shard", shardID)),
		token:   token,
		shardID: shardID,
		shards:  shards,

		// start with a 1kb buffer
		buf:  bytes.NewBuffer(make([]byte, 0, 1<<10)),
		hbuf: bytes.NewBuffer(nil),

		etcd: etcdCli,

		state: c,
		rc:    rdb,
	}

	sess.loadSessID()
	sess.loadSeq()
	return sess, nil
}

func (s *Session) initEtcd() error {
	sess, err := concurrency.NewSession(s.etcd, concurrency.WithContext(s.ctx), concurrency.WithTTL(20))
	if err != nil {
		return xerrors.Errorf("failed to get etcd session: %w", err)
	}

	s.etcdSess = sess
	s.identifyMu = concurrency.NewMutex(sess, IdentifyMutexName)
	return nil
}

func (s *Session) shouldResume() bool {
	return s.seq != 0 && s.sessID != ""
}

func (s *Session) Open(ctx context.Context, token string) error {
	s.wg.Add(1)
	defer s.wg.Done()

	s.ctx, s.cancel = context.WithCancel(ctx)
	defer s.cancel()

	s.lastAck = time.Time{}

	err := s.initEtcd()
	if err != nil {
		return err
	}

	// only acquire the identify lock if we know we won't send a resume
	if !s.shouldResume() {
		s.log.Debug("acquiring lock, no ability to resume")
		err = s.acquireIdentifyLock()
		if err != nil {
			return xerrors.Errorf("failed to grab identify lock: %w", err)
		}
		s.log.Debug("lock acquired")

	} else {
		s.log.Debug("skipping lock, attempting resume", zap.String("sess", s.sessID), zap.Int64("seq", s.seq))
	}

	r, err := czlib.NewReader(bytes.NewReader(nil))
	if err != nil {
		return xerrors.Errorf("failed to initialize zlib: %w", err)
	}
	s.zr = r
	defer r.Close()

	c, _, err := websocket.Dial(s.ctx, GatewayETF, nil)
	if err != nil {
		return xerrors.Errorf("failed to dial gateway: %w", err)
	}
	s.wsConn = c
	s.wsConn.SetReadLimit(512 << 20)

	err = s.readHello()
	if err != nil {
		return xerrors.Errorf("failed to handle hello message: %w", err)
	}

	if s.shouldResume() {
		s.log.Info("sending resume")
		err := s.writeResume()
		if err != nil {
			return xerrors.Errorf("failed to send resume: %w", err)
		}
	} else {
		s.last = 0
		s.log.Info("sending identify")
		err := s.writeIdentify()
		if err != nil {
			return xerrors.Errorf("failed to send identify: %w", err)
		}
	}

	go s.sendHeartbeats()
	go s.logTotalEvents()

	s.log.Info("websocket connected, waiting for events")

	for {
		err = s.readMessage()
		if err != nil {
			var werr websocket.CloseError
			if xerrors.Is(err, &werr) {
				if werr.Code == 4006 {
					s.seq = 0
					s.persistSeq()
					s.sessID = ""
					s.persistSessID()
				}
			}

			err = xerrors.Errorf("failed to read message: %w", err)
			break
		}

		var ev *discordetf.Event
		ev, err = discordetf.DecodeT(s.buf.Bytes())
		if err != nil {
			err = xerrors.Errorf("failed to decode event: %w", err)
			break
		}

		if ev.S != 0 {
			atomic.StoreInt64(&s.seq, ev.S)
		}

		if handled, err := s.handleInternalEvent(ev); handled {
			if err != nil {
				return err
			}

			continue
		}

		if ev.T == "PRESENCE_UPDATE" {
			continue
		}

		err = s.state.HandleEvent(ev)
		if err != nil {
			s.log.Error("failed to handle state event", zap.Error(err))
			continue
		}

		err = s.rc.RPush("gateway:events:"+ev.T, ev.D).Err()
		if err != nil {
			s.log.Error("failed to push event to redis", zap.Error(err))
		}
	}

	s.persistSeq()
	_ = c.Close(websocket.StatusNormalClosure, "")
	return err
}

func (s *Session) persistSeq() {
	err := s.rc.Set(s.fmtSeqKey(), s.seq, 0).Err()
	if err != nil && !xerrors.Is(err, redis.Nil) {
		s.log.Error("failed to save seq", zap.Error(err))
	}
}

func (s *Session) loadSeq() {
	sess, err := s.rc.Get(s.fmtSeqKey()).Result()
	if err != nil && !xerrors.Is(err, redis.Nil) {
		s.log.Error("failed to load session id", zap.Error(err))
	}

	if sess == "" {
		return
	}

	s.seq, err = strconv.ParseInt(sess, 10, 64)
	if err != nil {
		s.log.Error("failed to parse session id", zap.Error(err))
	}
}

func (s *Session) persistSessID() {
	err := s.rc.Set(s.fmtSessIDKey(), s.sessID, 0).Err()
	if err != nil && !xerrors.Is(err, redis.Nil) {
		s.log.Error("failed to save seq", zap.Error(err))
	}
}

func (s *Session) loadSessID() {
	sess, err := s.rc.Get(s.fmtSessIDKey()).Result()
	if err != nil && !xerrors.Is(err, redis.Nil) {
		s.log.Error("failed to load session id", zap.Error(err))
	}

	s.sessID = sess
}

func (s *Session) fmtSeqKey() string {
	return "gateway:seq:" + strconv.Itoa(s.shardID)
}

func (s *Session) fmtSessIDKey() string {
	return "gateway:sess:" + strconv.Itoa(s.shardID)
}

func (s *Session) handleInternalEvent(ev *discordetf.Event) (bool, error) {
	switch ev.Op {
	case 1:
		err := s.heartbeat()
		if err != nil {
			return true, xerrors.Errorf("failed to heartbeat in response to op 1: %w", err)
		}

	// RESUME
	case 6:
		s.log.Info("resumed")

		return true, nil

	// RECONNECT
	case 7:
		s.log.Info("reconnect requested")

		return true, xerrors.New("reconnect")

	// INVALID_SESSION
	case 9:
		s.log.Info("invalid session, reconnecting")
		s.sessID = ""
		s.persistSessID()
		s.seq = 0
		s.persistSeq()

		if s.identifyMu.IsOwner().Result == etcdserverpb.Compare_EQUAL {
			err := s.releaseIdentifyLock()
			if err != nil {
				s.log.Error("failed to release held identify lock after invalid session", zap.Error(err))
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
		_, sess, err := discordetf.DecodeReady(ev.D)
		if err != nil {
			return true, xerrors.Errorf("failed to decode ready: %w", err)
		}

		s.sessID = sess
		s.log.Info("ready", zap.String("sess", sess))
		s.persistSessID()

		go func() {
			time.Sleep(7 * time.Second)
			err = s.releaseIdentifyLock()
			if err != nil {
				s.log.Error("failed to release identify lock after ready", zap.Error(err))
			}
		}()

		return true, nil

	case "RESUMED":
		s.log.Info("resumed")
	}

	return false, nil
}

func (s *Session) acquireIdentifyLock() error {
	err := s.identifyMu.Lock(s.ctx)
	if err != nil {
		return xerrors.Errorf("failed to acquire identify lock: %w", err)
	}

	return nil
}

func (s *Session) releaseIdentifyLock() error {
	err := s.identifyMu.Unlock(s.ctx)
	if err != nil {
		return xerrors.Errorf("failed to release identify lock: %w", err)
	}

	return nil
}
