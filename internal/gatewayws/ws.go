package gatewayws

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
	"github.com/tatsuworks/gateway/state"
	"go.uber.org/zap"

	"github.com/pkg/errors"
	"github.com/valyala/bytebufferpool"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/discordetf"
	"github.com/tatsuworks/gateway/etf"
)

var (
	GatewayETF  = "wss://gateway.discord.gg?encoding=etf"
	GatewayJSON = "wss://gateway.discord.gg?encoding=json&compress=zlib-stream"
)

type Session struct {
	ctx    context.Context
	cancel func()

	log *zap.Logger

	token           string
	shardID, shards int

	seq    int64
	sessID string
	last   int64

	wsConn *websocket.Conn

	interval time.Duration
	trace    string

	bufs *bytebufferpool.Pool

	zlr io.ReadCloser

	state *state.Client

	rc *redis.Client
}

func NewSession(logger *zap.Logger, token string, shardID, shards int) (*Session, error) {
	return &Session{
		log:     logger,
		token:   token,
		shardID: shardID,
		shards:  shards,

		bufs: &bytebufferpool.Pool{},
	}, nil
}

func (s *Session) logTotalEvents() {
	for {
		time.Sleep(5 * time.Second)
		seq := atomic.LoadInt64(&s.seq)

		s.log.Info(
			"event report",
			zap.Int("shard", s.shardID),
			zap.Int64("seq", seq),
			zap.Int64("since", seq-s.last),
			zap.Float64("/sec", float64(seq-s.last)/5),
		)

		s.last = seq
	}
}

func (s *Session) Open(ctx context.Context, token string) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	c, _, err := websocket.Dial(s.ctx, GatewayETF, websocket.DialOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to dial gateway")
	}
	s.wsConn = c

	err = s.readHello()
	if err != nil {
		return errors.Wrap(err, "failed to handle hello message")
	}

	if s.seq == 0 && s.sessID == "" {
		err := s.writeIdentify()
		if err != nil {
			return errors.Wrap(err, "failed to send identify")
		}

	} else {
		err := s.writeResume()
		if err != nil {
			return errors.Wrap(err, "failed to send resume")
		}
	}

	byt, err := s.readMessage()
	if err != nil {
		return errors.Wrap(err, "failed to read message")
	}

	e, err := discordetf.DecodeT(byt)
	if err != nil {
		return errors.Wrap(err, "failed to decode event")
	}

	if e.T == "READY" {
		_, sess, err := discordetf.DecodeReady(e.D)
		if err != nil {
			return errors.Wrap(err, "failed to decode ready")
		}

		s.sessID = sess
		s.log.Info("ready", zap.Int("shard", s.shardID))

	} else if e.T == "RESUMED" {
		s.log.Info("resumed", zap.Int("shard", s.shardID))
	}

	go s.sendHeartbeats()
	go s.logTotalEvents()

	s.log.Info("websocket connected", zap.Int("shard", s.shardID))

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
		} else {
			// s.log.Info("0 seq received", zap.Int("shard", s.shardID), zap.String("type", ev.T), zap.Int("op", ev.Op))
		}

		start := time.Now()
		err = s.state.HandleEvent(ev)
		if err != nil {
			s.log.Error("failed to send event to state", zap.Error(err))
			os.Exit(0)
		}

		done := time.Since(start)
		_ = done
		// s.log.Info("sent event", zap.String("type", ev.T), zap.String("since", done.String()))

		_ = strings.ToLower
		// err = s.rc.RPush("gateway:events:"+strings.ToLower(ev.T), ev.D).Err()
		// if err != nil {
		// 	s.log.Error("failed to push event to redis", zap.Error(err))
		// }

		s.putRawBuf(byt)
	}

	c.Close(websocket.StatusNormalClosure, "")
	return err
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

	// if s.zlr == nil {
	// 	s.zlr, err = zlib.NewReader(r)
	// 	if err != nil {
	// 		return nil, errors.Wrap(err, "failed to create zlib reader")
	// 	}
	// } else {
	// 	err = s.zlr.(zlib.Resetter).Reset(r, nil)
	// 	if err != nil {
	// 		return nil, errors.Wrap(err, "failed to reset zlib reader")
	// 	}
	// }

	raw := s.bufs.Get()
	_, err = io.Copy(raw, r)
	if err != nil {
		s.bufs.Put(raw)
		return nil, errors.Wrap(err, "failed to copy hello")
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
			Compress:       false,
			LargeThreshold: 250,
			Shard:          []int{s.shardID, s.shards},
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
		return errors.Wrap(err, "failed to copy heartbear")
	}

	return nil
}

func (s *Session) sendHeartbeats() {
	t := time.NewTicker(s.interval)

	for {
		err := s.heartbeat()
		if err != nil {
			fmt.Println(err)
		}

		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
		}
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
	Token          string `json:"token"`
	Properties     props  `json:"properties"`
	Compress       bool   `json:"compress"`
	LargeThreshold int    `json:"large_threshold"`
	Shard          []int  `json:"shard"`
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
