package gatewayws

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"io"
	"io/ioutil"
	"nhooyr.io/websocket"
	"time"

	"github.com/fngdevs/gateway/discordetf"
	"github.com/pkg/errors"
	"github.com/valyala/bytebufferpool"
)

var (
	GatewayETF  = "wss://gateway.discord.gg?encoding=etf"
	GatewayJSON = "wss://gateway.discord.gg?encoding=json&compress=zlib-stream"
)

type Session struct {
	ctx    context.Context
	cancel func()

	seq    int64
	wsConn *websocket.Conn

	interval time.Duration
	trace    string

	bufs *bytebufferpool.Pool
}

func NewSession() *Session {
	return &Session{
		bufs: &bytebufferpool.Pool{},
	}
}

func (s *Session) Open(ctx context.Context, token string) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	c, _, err := websocket.Dial(s.ctx, GatewayETF)
	if err != nil {
		return errors.Wrap(err, "failed to dial gateway")
	}
	s.wsConn = c

	err = s.readHello()
	if err != nil {
		return errors.Wrap(err, "failed to handle hello message")
	}

	err = s.writeIdentify()
	if err != nil {
		return errors.Wrap(err, "failed to write identify payload")
	}

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

		spew.Dump(ev)
	}

	c.Close(websocket.StatusNormalClosure, "")
	return err
}

func (s *Session) writeIdentify() error {
	w, err := s.wsConn.Write(s.ctx, websocket.MessageBinary)
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

	err = w.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close identify writer")
	}

	return nil
}

func (s *Session) readHello() error {
	_, r, err := s.wsConn.Read(s.ctx)
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
	_, r, err := s.wsConn.Read(s.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reader")
	}

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
	// var (
	// 	buf = s.bufs.Get()
	// 	c   = new(etf.Context)
	// )

	// err := c.Write(buf, identifyOp{
	// 	Op: 2,
	// 	Data: identify{
	// 		Token: "Bot ",
	// 		Properties: props{
	// 			Os:      runtime.GOOS,
	// 			Browser: "https://github.com/fngdevs/gateway",
	// 			Device:  "Go",
	// 		},
	// 		Compress:       true,
	// 		LargeThreshold: 250,
	// 	},
	// })

	b, err := ioutil.ReadFile("rawetf")
	return b, errors.Wrap(err, "failed to write identify payload")
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
}

type props struct {
	Os              string `json:"$os"`
	Browser         string `json:"$browser"`
	Device          string `json:"$device"`
	Referer         string `json:"$referer"`
	ReferringDomain string `json:"$referring_domain"`
}
