package gatewayws

import (
	"encoding/json"
	"runtime"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
)

type Op struct {
	Op int         `json:"op"`
	D  interface{} `json:"d"`
}

func (s *Session) writer() {
	var (
		ctx    = s.ctx
		wch    = s.wch
		prioch = s.prioch
		isPrio bool
	)

	for {
		var msg *Op
		select {
		case <-ctx.Done():
			return
		// we always check the prio channel first since that should
		// take precedence over other messages
		case msg = <-prioch:
			isPrio = true
		case msg = <-wch:
			if !s.authed {
				wch <- msg
				time.Sleep(25 * time.Millisecond)
				continue
			}
		}

		err := s.rl.Wait(ctx)
		if err != nil {
			return
		}

		err = s.writeOp(msg)
		if err != nil {
			if !isPrio {
				wch <- msg
			}
			s.log.Error(s.ctx, "write ws message", slog.Error(err), slog.F("op", msg.Op))
			return
		}
		isPrio = false
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *Session) writeOp(op *Op) error {
	raw, err := s.enc.Write(*op)
	if err != nil {
		return xerrors.Errorf("encode op: %w", err)
	}

	w, err := s.wsConn.Writer(s.ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("get writer: %w", err)
	}
	defer w.Close()

	_, err = w.Write(raw)
	if err != nil {
		return xerrors.Errorf("write payload: %w", err)
	}

	return nil
}

type Identify struct {
	Token          string `json:"token"`
	Properties     Props  `json:"properties"`
	Compress       bool   `json:"compress"`
	LargeThreshold int    `json:"large_threshold"`
	Shard          []int  `json:"shard"`
	Intents        int    `json:"intents,omitempty"`
}

type Props struct {
	Os              string `json:"$os"`
	Browser         string `json:"$browser"`
	Device          string `json:"$device"`
	Referer         string `json:"$referer"`
	ReferringDomain string `json:"$referring_domain"`
}

func (s *Session) writeIdentify() {
	s.prioch <- &Op{
		Op: 2,
		D: Identify{
			Token: s.token,
			Properties: Props{
				Os:      runtime.GOOS,
				Browser: "https://github.com/tatsuworks/gateway",
				Device:  runtime.Version(),
			},
			Compress:       false,
			LargeThreshold: 250,
			Shard:          []int{s.shardID, s.shardCount},
			Intents:        s.intents.Collect(),
		},
	}
}

type Resume struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Sequence  int64  `json:"seq"`
}

func (s *Session) writeResume() {
	s.prioch <- &Op{
		Op: 6,
		D: Resume{
			Token:     s.token,
			SessionID: s.sessID,
			Sequence:  s.seq,
		},
	}
}

func (s *Session) writeHeartbeat() {
	s.prioch <- &Op{
		Op: 1,
		D:  atomic.LoadInt64(&s.seq),
	}
}

func (s *Session) listenOpCodes() {
	var (
		t   = time.NewTicker(time.Second)
		ctx = s.ctx
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ssCmd := s.rc.BLPop(time.Second, "gateway:op")
		if ssCmd.Err() != nil {
			continue
		}
		opjson := ssCmd.String()
		s.log.Info(ctx, "opjson", slog.F("opjson", opjson))
		var op Op
		err := json.Unmarshal([]byte(opjson), &op)
		if err != nil {
			continue
		}
		s.log.Info(ctx, "unmarshaled", slog.F("op", op))
		s.prioch <- &op
	}
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
			if s.lastAck.Sub(s.lastHB) >= s.interval {
				s.log.Warn(s.ctx, "no response to heartbeat")
				cancel()
				return
			}
		}

		s.writeHeartbeat()
		s.lastHB = time.Now()
	}
}

type RequestGuildMembers struct {
	GuildID int64  `json:"guild_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

func (s *Session) requestGuildMembers(guild int64) {
	select {
	case s.wch <- &Op{
		Op: 8,
		D: RequestGuildMembers{
			GuildID: guild,
		},
	}:
	default:
		s.log.Error(s.ctx, "write channel full")
	}

}

type status struct {
	Game   game   `json:"game"`
	Status string `json:"status"`
}

type game struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

func (s *Session) rotateStatuses() {
	var (
		ctx      = s.ctx
		statuses = []string{
			"Use t!help",
			"https://tatsumaki.xyz",
		}
	)

	time.Sleep(10 * time.Second)

	for {
		for _, e := range statuses {
			select {
			case <-ctx.Done():
				return
			default:
			}

			s.log.Debug(s.ctx, "writing status", slog.F("status", e))

			s.wch <- &Op{
				Op: 3,
				D: status{
					Game: game{
						Name: e,
						Type: 0,
					},
					Status: "online",
				},
			}
			time.Sleep(time.Minute)
		}
	}

}

func (s *Session) RequestGuildMembers(guildID int64) {
	op := &Op{
		Op: 8,
		D: RequestGuildMembers{
			GuildID: guildID,
		},
	}

	s.log.Info(s.ctx, "sending members request", slog.F("guild", guildID))
	s.wch <- op
}
