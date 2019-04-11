package gatewayws

import (
	"context"
	"net"

	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/ws"
	"github.com/pkg/errors"
)

var (
	Gateway = "wss://gateway.discord.gg?encoding=etf"
)

type Session struct {
	ctx    context.Context
	cancel func()

	seq    int64
	wsConn net.Conn
}

func NewSession() *Session {
	return &Session{}
}

func (s *Session) Open(ctx context.Context, token string) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	conn, _, _, err := ws.Dial(s.ctx, Gateway)
	if err != nil {
		return errors.Wrap(err, "failed to open websocket")
	}

	f, err := ws.ReadFrame(conn)
	if err != nil {
		return errors.Wrap(err, "failed to read frame")
	}

	spew.Dump(f)
	return nil
}
