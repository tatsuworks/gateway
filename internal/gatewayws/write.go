package gatewayws

import (
	"context"
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

	// A dead writer means the connection is unusable. Cancel it on exit so the
	// read loop and sendHeartbeats can't block forever on the unbuffered
	// prioch/wch sends (nothing else drains them once writer is gone).
	defer s.Cancel()

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
			s.log.Error(s.ctx, "write ws message", slog.Error(err), slog.F("op", msg.Op))
			if !isPrio {
				s.requeue(msg)
			}
			return
		}
		isPrio = false
		time.Sleep(25 * time.Millisecond)
	}
}

// requeue best-effort returns a failed non-priority op to the write queue for
// the next connection. It never blocks: writer() is the only drainer of wch, so
// a blocking send on a full queue would wedge writer() before its deferred
// s.Cancel() runs. If the queue is full the op is dropped — the connection is
// being torn down anyway and presence/RGM ops are re-driven on reconnect.
func (s *Session) requeue(op *Op) {
	select {
	case s.wch <- op:
	default:
		s.log.Error(s.ctx, "dropping op; write queue full during teardown", slog.F("op", op.Op))
	}
}

func (s *Session) writeOp(op *Op) error {
	raw, err := s.enc.Write(*op)
	if err != nil {
		return xerrors.Errorf("encode op: %w", err)
	}

	// Bound the write the same way readMessage bounds the read. Without a
	// deadline a black-holed socket (remote stopped reading, send buffer full)
	// blocks here indefinitely: the writer never returns, never reaches its
	// defer s.Cancel(), and the read loop wedges on the unbuffered prioch send
	// inside writeHeartbeat — the "handle internal event " freeze seen in prod.
	// With the deadline a stuck write fails, the writer exits and cancels the
	// connection, and the read loop unwinds and reconnects.
	ctx, cancel := context.WithTimeout(s.ctx, connectionTimeout*time.Second)
	defer cancel()

	w, err := s.wsConn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("get writer: %w", err)
	}

	if _, err := w.Write(raw); err != nil {
		w.Close()
		return xerrors.Errorf("write payload: %w", err)
	}

	// Close writes the FIN frame and flushes to the socket — for small messages
	// that final flush (and a deadline expiry on a black-holed connection) lands
	// here, not in Write. A deferred Close would discard the error and writeOp
	// would return nil, so writer() would treat a dropped op as delivered and
	// spin on a dead connection instead of exiting and cancelling. Propagate it.
	if err := w.Close(); err != nil {
		return xerrors.Errorf("flush payload: %w", err)
	}

	return nil
}

type Identify struct {
	Token          string         `json:"token"`
	Properties     Props          `json:"properties"`
	Compress       bool           `json:"compress"`
	LargeThreshold int            `json:"large_threshold"`
	Shard          []int          `json:"shard"`
	Intents        int            `json:"intents,omitempty"`
	Presence       updatePresence `json:"presence"`
}

type Props struct {
	Os      string `json:"os"`
	Browser string `json:"browser"`
	Device  string `json:"device"`
}
type updatePresence struct {
	Activities []*activity `json:"activities"`
	Status     string      `json:"status"`
	AFK        bool        `json:"afk"`
}

func (s *Session) writeIdentify() {
	select {
	case <-s.ctx.Done():
		return
	case s.prioch <- &Op{
		Op: 2,
		D: Identify{
			Token: s.token,
			Properties: Props{
				Os:      runtime.GOOS,
				Browser: "https://github.com/tatsuworks/gateway",
				Device:  runtime.Version(),
			},
			Compress:       false,
			LargeThreshold: LargeThreshold,
			Shard:          []int{s.shardID, s.shardCount},
			Intents:        s.intents.Collect(),
			Presence: updatePresence{
				Activities: []*activity{
					{
						Name: "https://tatsu.gg",
						Type: 0,
					},
				},
				Status: "online",
			},
		},
	}:
	}
}

type Resume struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Sequence  int64  `json:"seq"`
}

func (s *Session) writeResume() {
	select {
	case <-s.ctx.Done():
	case s.prioch <- &Op{
		Op: 6,
		D: Resume{
			Token:     s.token,
			SessionID: s.sessID,
			Sequence:  atomic.LoadInt64(&s.seq),
		},
	}:
	}
}

func (s *Session) writeHeartbeat() {
	// Abort if the connection is being torn down. prioch is unbuffered, so a bare
	// send wedges the read loop forever once writer() has exited.
	select {
	case s.prioch <- &Op{
		Op: 1,
		D:  atomic.LoadInt64(&s.seq),
	}:
	case <-s.ctx.Done():
	}
}

// resetHeartbeat clears heartbeat tracking for a new connection. Both timestamps
// must be cleared together: a Session is reused across reconnects, so a lastHB
// left over from the previous connection (while lastAck is reset to zero) would
// make heartbeatStale fire on the first tick and cancel the fresh connection
// before it ever heartbeats — a reconnect loop.
func (s *Session) resetHeartbeat() {
	s.lastHB = time.Time{}
	s.lastAck = time.Time{}
}

// heartbeatStale reports whether the heartbeat we most recently sent has gone
// unacked — the gateway has stopped responding and the connection is a zombie.
// It is evaluated once per interval on the heartbeat ticker, so an unacked
// lastHB means a full interval elapsed with no ACK in response.
//
// It compares lastAck to lastHB rather than measuring now-minus-lastHB on
// purpose. The ticker fires on a fixed cadence, but lastHB is recorded slightly
// after each tick (the writer's post-write sleep / an in-flight write delay it),
// so a now-minus-lastHB >= interval check always read just under interval and
// could never fire — leaving the zombie connection wedged. Comparing timestamps
// is timing-robust: lastAck not strictly after lastHB means the last heartbeat
// is still unacked at this tick.
func (s *Session) heartbeatStale() bool {
	if s.lastHB.IsZero() {
		return false
	}
	return !s.lastAck.After(s.lastHB)
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

		if s.heartbeatStale() {
			s.log.Warn(s.ctx, "no response to heartbeat; tearing down connection",
				slog.F("last_hb", s.lastHB), slog.F("last_ack", s.lastAck))
			cancel()
			return
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

type activity struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

func (s *Session) rotateStatuses() {
	var (
		ctx      = s.ctx
		statuses = []string{
			"Use t!help",
			"https://tatsu.gg",
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
				D: updatePresence{
					Activities: []*activity{
						{
							Name: e,
							Type: 0,
						},
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
	select {
	case s.wch <- op:
	case <-s.ctx.Done():
	}
}
