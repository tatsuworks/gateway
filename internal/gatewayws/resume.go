package gatewayws

import (
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/etf"
)

type resumeOp struct {
	Op   int    `json:"op"`
	Data resume `json:"d"`
}

type resume struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Sequence  int64  `json:"seq"`
}

func (s *Session) writeResume() error {
	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("failed to get writer: %w", err)
	}

	err = s.resumePayload()
	if err != nil {
		return err
	}

	_, err = w.Write(s.buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to write identify payload: %w", err)
	}

	if err := w.Close(); err != nil {
		return xerrors.Errorf("failed to close identify writer: %w", err)
	}

	return nil
}

func (s *Session) resumePayload() error {
	var c = new(etf.Context)

	s.buf.Reset()
	err := c.Write(s.buf, resumeOp{
		Op: 6,
		Data: resume{
			Token:     s.token,
			SessionID: s.sessID,
			Sequence:  s.seq,
		},
	})
	if err != nil {
		return xerrors.Errorf("failed to marshal resume payload: %w", err)
	}

	return nil
}
