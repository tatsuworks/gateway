package gatewayws

import (
	"bytes"
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/discordetf"
	"github.com/tatsuworks/gateway/handler"
)

var (
	GatewayETF = "wss://gateway.discord.gg?encoding=etf"
)

type Session struct {
	ctx    context.Context
	cancel func()

	log *zap.Logger

	token   string
	shardID int
	shards  int

	seq    int64
	sessID string
	last   int64

	wsConn *websocket.Conn

	interval time.Duration
	trace    string

	lastHB  time.Time
	lastAck time.Time

	buf  *bytes.Buffer
	hbuf *bytes.Buffer

	state *handler.Client
	rc    *redis.Client
}

func NewSession(logger *zap.Logger, rdb *redis.Client, token string, shardID, shards int) (*Session, error) {
	c, err := handler.NewClient()
	if err != nil {
		return nil, xerrors.Errorf("failed to create state handler: %w", err)
	}

	return &Session{
		log:     logger.With(zap.Int("shard", shardID)),
		token:   token,
		shardID: shardID,
		shards:  shards,

		// start with a 1kb buffer
		buf:  bytes.NewBuffer(make([]byte, 0, 1<<10)),
		hbuf: bytes.NewBuffer(nil),

		state: c,
		rc:    rdb,
	}, nil
}

func (s *Session) Open(ctx context.Context, token string, connected chan struct{}) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	defer s.cancel()
	s.lastAck = time.Time{}

	c, _, err := websocket.Dial(s.ctx, GatewayETF, nil)
	if err != nil {
		return xerrors.Errorf("failed to dial gateway: %w", err)
	}
	s.wsConn = c
	s.wsConn.SetReadLimit(999999999)

	err = s.readHello()
	if err != nil {
		return xerrors.Errorf("failed to handle hello message: %w", err)
	}

	if s.seq == 0 && s.sessID == "" {
		s.log.Debug("sending identify")
		err := s.writeIdentify()
		if err != nil {
			return xerrors.Errorf("failed to send identify: %w", err)
		}

	} else {
		s.log.Debug("sending resume")
		err := s.writeResume()
		if err != nil {
			return xerrors.Errorf("failed to send resume: %w", err)
		}
	}

	go s.sendHeartbeats()
	go s.logTotalEvents()

	s.log.Info("websocket connected")

	for {
		err = s.readMessage()
		if err != nil {
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
			select {
			case <-connected:
			default:
				close(connected)
			}

			if err != nil {
				return err
			}

			continue
		}

		err = s.state.HandleEvent(ev)
		if err != nil {
			s.log.Error("failed to handle state event", zap.Error(err))
			continue
		}

		_, err = s.rc.Pipelined(func(pipe redis.Pipeliner) error {
			if err := pipe.Set("gateway:seq:"+strconv.Itoa(s.shardID), s.seq, 0).Err(); err != nil {
				return xerrors.Errorf("failed to set seq in redis: %w", err)
			}

			if ev.T == "PRESENCE_UPDATE" {
				return nil
			}

			if err := pipe.RPush("gateway:events:"+ev.T, ev.D).Err(); err != nil {
				return xerrors.Errorf("failed to push event to redis: %w", err)
			}

			return nil
		})
		if err != nil {
			s.log.Error("failed to run event pipeline", zap.Error(err))
		}
	}

	_ = c.Close(websocket.StatusNormalClosure, "")
	return err
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
		s.seq = 0

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
		s.log.Info("ready")

		return true, nil

	case "PRESENCE_UPDATE":
		return false, nil
	}

	return false, nil
}
