package gatewayws

import (
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

	rawIdentify, err := s.identifyPayload()
	if err != nil {
		return xerrors.Errorf("failed to make identify payload: %w", err)
	}

	_, err = w.Write(rawIdentify)
	if err != nil {
		return xerrors.Errorf("failed to write identify payload: %w", err)
	}

	if err := w.Close(); err != nil {
		return xerrors.Errorf("failed to close identify writer: %w", err)
	}

	return nil
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
	if err != nil {
		return nil, xerrors.Errorf("failed to write identify payload: %w", err)
	}

	return buf.B, nil
}
