package gatewayws

import (
	"bytes"
	"runtime"

	"golang.org/x/xerrors"
	"nhooyr.io/websocket"

	"github.com/tatsuworks/gateway/etf"
)

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

type props struct {
	Os              string `json:"$os"`
	Browser         string `json:"$browser"`
	Device          string `json:"$device"`
	Referer         string `json:"$referer"`
	ReferringDomain string `json:"$referring_domain"`
}

func (s *Session) writeIdentify() error {
	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("failed to get writer: %w", err)
	}

	err = s.identifyPayload()
	if err != nil {
		return xerrors.Errorf("failed to make identify payload: %w", err)
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

func (s *Session) identifyPayload() error {
	var c = new(etf.Context)

	s.buf.Reset()
	err := c.Write(s.buf, identifyOp{
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
	if err != nil {
		return xerrors.Errorf("failed to write identify payload: %w", err)
	}

	return nil
}

type op struct {
	Op int         `json:"op"`
	D  interface{} `json:"d"`
}

type requestGuildMembers struct {
	GuildID int64  `json:"guild_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

func (s *Session) requestGuildMembers(guild int64) error {
	var (
		c   = new(etf.Context)
		buf = new(bytes.Buffer)
	)

	err := c.Write(buf, op{
		Op: 8,
		D: requestGuildMembers{
			GuildID: guild,
			Query:   "",
			Limit:   0,
		},
	})
	if err != nil {
		return xerrors.Errorf("failed to encode guild member request: %w", err)
	}

	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("failed to get writer: %w", err)
	}

	_, err = w.Write(buf.Bytes())
	if err != nil {
		return xerrors.Errorf("failed to write guild member request payload: %w", err)
	}

	if err := w.Close(); err != nil {
		return xerrors.Errorf("failed to close guild member request writer: %w", err)
	}

	return nil
}
