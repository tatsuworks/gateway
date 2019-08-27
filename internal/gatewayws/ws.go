package gatewayws

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"github.com/valyala/bytebufferpool"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/discordetf"
	"github.com/tatsuworks/gateway/etf"
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

	bufs *bytebufferpool.Pool

	zlr io.ReadCloser

	state *handler.Client

	rc *redis.Client
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

		bufs: &bytebufferpool.Pool{},

		state: c,
		rc:    rdb,
	}, nil
}

func (s *Session) Open(ctx context.Context, token string, connected chan struct{}) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	defer s.cancel()
	s.lastAck = time.Time{}

	c, _, err := websocket.Dial(s.ctx, GatewayETF, websocket.DialOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to dial gateway")
	}
	s.wsConn = c
	s.wsConn.SetReadLimit(999999999)

	err = s.readHello()
	if err != nil {
		return errors.Wrap(err, "failed to handle hello message")
	}

	if s.seq == 0 && s.sessID == "" {
		s.log.Debug("sending ready")
		err := s.writeIdentify()
		if err != nil {
			return errors.Wrap(err, "failed to send identify")
		}

	} else {
		s.log.Debug("sending resume")
		err := s.writeResume()
		if err != nil {
			return errors.Wrap(err, "failed to send resume")
		}
	}

	go s.sendHeartbeats()
	go s.logTotalEvents()

	s.log.Info("websocket connected")

	for {
		var byt []byte
		byt, err = s.readMessage()
		if err != nil {
			err = errors.Wrap(err, "failed to read message")
			break
		}

		var ev *discordetf.Event
		ev, err = discordetf.DecodeT(byt)
		if err != nil {
			err = errors.Wrap(err, "failed to decode event")
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

			s.putRawBuf(byt)
			continue
		}

		err = s.state.HandleEvent(ev)
		if err != nil {
			s.log.Error("failed to send event to state", zap.Error(err))
			continue
		}

		_, err = s.rc.Pipelined(func(pipe redis.Pipeliner) error {
			if err := pipe.Set(fmt.Sprintf("gateway:seq:%d", s.shardID), s.seq, 0).Err(); err != nil {
				return xerrors.Errorf("failed to set seq in redis: %w", err)
			}

			if err := pipe.RPush("gateway:events:"+ev.T, ev.D).Err(); err != nil {
				return xerrors.Errorf("failed to push event to redis: %w", err)
			}

			return nil
		})
		if err != nil {
			s.log.Error("failed to run event pipeline", zap.Error(err))
		}

		s.putRawBuf(byt)
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
		s.log.Info("heartbeat ack received")
		s.lastAck = time.Now()
		return true, nil
	}

	switch ev.T {
	case "READY":
		_, sess, err := discordetf.DecodeReady(ev.D)
		if err != nil {
			return true, errors.Wrap(err, "failed to decode ready")
		}

		s.sessID = sess
		s.log.Info("ready")

		return true, nil

	case "PRESENCE_UPDATE":
		return false, nil
	}

	s.log.Info("event received", zap.String("type", ev.T))
	return false, nil
}

func (s *Session) writeIdentify() error {
	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	rawIdentify, err := s.identifyPayload()
	if err != nil {
		return errors.Wrap(err, "failed to make identify payload")
	}

	_, err = w.Write(rawIdentify)
	if err != nil {
		return errors.Wrap(err, "failed to write identify payload")
	}

	return errors.Wrap(w.Close(), "failed to close identify writer")
}

func (s *Session) readHello() error {
	_, r, err := s.wsConn.Reader(s.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get hello reader")
	}

	raw := s.bufs.Get()
	_, err = io.Copy(raw, r)
	if err != nil {
		s.bufs.Put(raw)
		return errors.Wrap(err, "failed to copy hello")
	}

	interval, trace, err := discordetf.DecodeHello(raw.B)
	if err != nil {
		return errors.Wrap(err, "failed to decode hello message")
	}

	s.bufs.Put(raw)
	s.interval = time.Duration(interval) * time.Millisecond
	s.trace = trace

	return nil
}

func (s *Session) readMessage() ([]byte, error) {
	_, r, err := s.wsConn.Reader(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}

	raw := s.bufs.Get()
	_, err = io.Copy(raw, r)
	if err != nil {
		s.bufs.Put(raw)
		return nil, errors.Wrap(err, "failed to copy message")
	}

	return raw.B, nil
}

func (s *Session) putRawBuf(buf []byte) {
	s.bufs.Put(&bytebufferpool.ByteBuffer{B: buf})
}

func (s *Session) identifyPayload() ([]byte, error) {
	var (
		buf = s.bufs.Get()
		c   = new(etf.Context)
	)

	err := c.Write(buf, identifyOp{
		Op: 2,
		Data: identify{
			Token: s.token,
			Properties: props{
				Os:      runtime.GOOS,
				Browser: "https://github.com/tatsuworks/gateway",
				Device:  "Go",
			},
			Compress:           false,
			LargeThreshold:     250,
			GuildSubscriptions: true,
			Shard:              []int{s.shardID, s.shards},
		},
	})

	return buf.B, errors.Wrap(err, "failed to write identify payload")
}

func (s *Session) writeResume() error {
	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return errors.Wrap(err, "failed to get writer")
	}

	payload, err := s.resumePayload()
	if err != nil {
		return err
	}

	_, err = w.Write(payload)
	if err != nil {
		return errors.Wrap(err, "failed to write identify payload")
	}

	return errors.Wrap(w.Close(), "failed to close identify writer")
}

func (s *Session) resumePayload() ([]byte, error) {
	var (
		buf = s.bufs.Get()
		c   = new(etf.Context)
	)

	err := c.Write(buf, resumeOp{
		Op: 6,
		Data: resume{
			Token:     s.token,
			SessionID: s.sessID,
			Sequence:  s.seq,
		},
	})

	return buf.B, errors.Wrap(err, "failed to write resume payload")
}

func (s *Session) heartbeat() error {
	var (
		buf = s.bufs.Get()
		c   = new(etf.Context)
	)

	defer s.bufs.Put(buf)
	err := c.Write(buf, heartbeatOp{
		Op:   1,
		Data: atomic.LoadInt64(&s.seq),
	})
	if err != nil {
		return errors.Wrap(err, "failed to write heartbeat")
	}

	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return errors.Wrap(err, "failed to get heartbeat writer")
	}
	defer w.Close()

	_, err = w.Write(buf.B)
	if err != nil {
		return errors.Wrap(err, "failed to copy heartbeat")
	}

	return nil
}

func (s *Session) sendHeartbeats() {
	var (
		t      = time.NewTicker(s.interval)
		ctx    = s.ctx
		cancel = s.cancel
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		if !s.lastHB.IsZero() {
			if s.lastAck.Sub(s.lastHB) > 10*time.Second {
				s.log.Warn("no response to heartbeat, closing")
				cancel()
				continue
			}
		}

		err := s.heartbeat()
		if err != nil {
			s.log.Error("failed to send heartbeat", zap.Error(err))
		}

		s.log.Info("sent heartbeat")
		s.lastHB = time.Now()
	}
}

func (s *Session) logTotalEvents() {
	var (
		t   = time.NewTicker(15 * time.Second)
		ctx = s.ctx
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		seq := atomic.LoadInt64(&s.seq)

		if seq-s.last == 0 {
			s.log.Info(
				"event report",
				zap.Int64("seq", seq),
				zap.Int64("since", seq-s.last),
				zap.Float64("/sec", float64(seq-s.last)/15),
			)
		}

		s.last = seq
	}
}

type heartbeatOp struct {
	Op   int   `json:"op"`
	Data int64 `json:"d"`
}

type identifyOp struct {
	Op   int      `json:"op"`
	Data identify `json:"d"`
}

type identify struct {
	Token              string `json:"token"`
	Properties         props  `json:"properties"`
	Compress           bool   `json:"compress"`
	LargeThreshold     int    `json:"large_threshold"`
	GuildSubscriptions bool   `json:"guild_subscriptions"`
	Shard              []int  `json:"shard"`
}

type resumeOp struct {
	Op   int    `json:"op"`
	Data resume `json:"d"`
}

type resume struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Sequence  int64  `json:"seq"`
}

type props struct {
	Os              string `json:"$os"`
	Browser         string `json:"$browser"`
	Device          string `json:"$device"`
	Referer         string `json:"$referer"`
	ReferringDomain string `json:"$referring_domain"`
}
