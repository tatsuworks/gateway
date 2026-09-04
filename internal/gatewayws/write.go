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

func (c *conn) writer() {
	var (
		ctx    = c.ctx
		wch    = c.wch
		prioch = c.prioch
	)

	// A dead writer means the connection is unusable. Cancel it on exit so the
	// read loop and sendHeartbeats can't block forever on the unbuffered
	// prioch/wch sends (nothing else drains them once writer is gone).
	defer c.cancel()

	for {
		var msg *Op
		select {
		case <-ctx.Done():
			return
		// we always check the prio channel first since that should
		// take precedence over other messages
		case msg = <-prioch:
		case msg = <-wch:
			if !c.authed.Load() {
				// Not authed yet: hold the op until READY/RESUMED instead of
				// sending it pre-auth. The requeue is context-aware so a full
				// wch can't wedge the writer here (we are its only drainer)
				// before its deferred cancel runs.
				select {
				case wch <- msg:
				case <-ctx.Done():
					return
				}
				time.Sleep(25 * time.Millisecond)
				continue
			}
		}

		err := c.s.rl.Wait(ctx)
		if err != nil {
			return
		}

		err = c.writeOp(msg)
		if err != nil {
			// The connection is dead (write failed). Don't requeue: wch is
			// per-conn, so the next Open allocates a fresh one and a requeued
			// op is orphaned anyway — and a blocking requeue into a full wch
			// would wedge the writer before its deferred cancel runs, the exact
			// teardown stall this refactor removes. Log and exit; defer
			// c.cancel() tears the conn down and the read loop reconnects.
			c.s.log.Error(c.ctx, "write ws message", slog.Error(err), slog.F("op", msg.Op))
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (c *conn) writeOp(op *Op) error {
	raw, err := c.s.enc.Write(*op)
	if err != nil {
		return xerrors.Errorf("encode op: %w", err)
	}

	// Bound the write the same way readMessage bounds the read. Without a
	// deadline a black-holed socket (remote stopped reading, send buffer full)
	// blocks here indefinitely: the writer never returns, never reaches its
	// defer c.cancel(), and the read loop wedges on the unbuffered prioch send
	// inside writeHeartbeat — the "handle internal event " freeze seen in prod.
	// With the deadline a stuck write fails, the writer exits and cancels the
	// connection, and the read loop unwinds and reconnects.
	ctx, cancel := context.WithTimeout(c.ctx, connectionTimeout*time.Second)
	defer cancel()

	w, err := c.wsConn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return xerrors.Errorf("get writer: %w", err)
	}

	if _, err = w.Write(raw); err != nil {
		w.Close()
		return xerrors.Errorf("write payload: %w", err)
	}

	// Close flushes the final frame. Check its error rather than deferring it:
	// nhooyr surfaces a black-holed/timed-out write here, and swallowing it would
	// let writeOp return nil on a failed send so the writer never exits or cancels.
	if err = w.Close(); err != nil {
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

func (c *conn) writeIdentify() {
	select {
	case <-c.ctx.Done():
		return
	case c.prioch <- &Op{
		Op: 2,
		D: Identify{
			Token: c.s.token,
			Properties: Props{
				Os:      runtime.GOOS,
				Browser: "https://github.com/tatsuworks/gateway",
				Device:  runtime.Version(),
			},
			Compress:       false,
			LargeThreshold: LargeThreshold,
			Shard:          []int{c.s.shardID, c.s.shardCount},
			Intents:        c.s.intents.Collect(),
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

func (c *conn) writeResume() {
	select {
	case <-c.ctx.Done():
	case c.prioch <- &Op{
		Op: 6,
		D: Resume{
			Token:     c.s.token,
			SessionID: c.s.sessID,
			Sequence:  atomic.LoadInt64(&c.s.seq),
		},
	}:
	}
}

func (c *conn) writeHeartbeat() {
	// Abort if the connection is being torn down. prioch is unbuffered, so a bare
	// send wedges the read loop forever once writer() has exited.
	select {
	case c.prioch <- &Op{
		Op: 1,
		D:  atomic.LoadInt64(&c.s.seq),
	}:
	case <-c.ctx.Done():
	}
}

// heartbeatStale reports whether the most recent heartbeat we sent has gone
// unacked for at least one interval — i.e. the gateway has stopped responding
// and the connection is dead. It returns false until we have actually sent a
// heartbeat (lastHB zero) or while the latest send has been acked (lastAck at or
// after lastHB). Measuring "time since the unacked send" rather than
// lastAck.Sub(lastHB) is deliberate: once ACKs stop, lastAck falls behind lastHB
// and the old difference went negative, so the watchdog could never fire.
func (c *conn) heartbeatStale(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastHB.IsZero() {
		return false
	}
	return c.lastHB.After(c.lastAck) && now.Sub(c.lastHB) >= c.interval
}

func (c *conn) sendHeartbeats() {
	var (
		t      = time.NewTicker(c.interval)
		ctx    = c.ctx
		cancel = c.cancel
	)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		if c.heartbeatStale(time.Now()) {
			_, lastHB, lastAck, _ := c.snapshot()
			c.s.log.Warn(c.ctx, "no response to heartbeat; tearing down connection",
				slog.F("last_hb", lastHB), slog.F("last_ack", lastAck))
			cancel()
			return
		}

		c.writeHeartbeat()
		c.markHB(time.Now())
	}
}

type RequestGuildMembers struct {
	GuildID int64  `json:"guild_id"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

func (c *conn) requestGuildMembers(guild int64) {
	// If the connection is torn down the writer has already exited and the next
	// Open allocates a fresh wch, so a send here (this path is non-blocking and
	// the buffer usually has room) is silently orphaned. This path runs under
	// the parent ctx, so a disconnect mid-GUILD_CREATE still reaches here with a
	// dead conn. Drop + log instead. The guild is re-requested on the next
	// IDENTIFY (which replays GUILD_CREATE); a RESUME does not replay it, so a
	// guild dropped here stays un-backfilled until the next full IDENTIFY or a
	// manual RequestGuildMembers. (A background staleness sweep was designed to
	// cover this gap but is currently deferred/unbuilt.)
	if c.ctx.Err() != nil {
		c.s.log.Info(c.ctx, "drop guild member backfill: connection closed", slog.F("guild", guild))
		return
	}
	select {
	case c.wch <- &Op{
		Op: 8,
		D: RequestGuildMembers{
			GuildID: guild,
		},
	}:
	default:
		c.s.log.Error(c.ctx, "write channel full", slog.F("guild", guild))
	}

}

type activity struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

func (c *conn) rotateStatuses() {
	var (
		ctx      = c.ctx
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

			c.s.log.Debug(c.ctx, "writing status", slog.F("status", e))

			// ctx-guarded send: wch is per-conn and the writer is its only
			// drainer, so a bare send wedges this goroutine forever if the writer
			// has exited. (This path is currently disabled — see the commented
			// `go c.rotateStatuses()` in run — but keep it safe-by-construction so
			// re-enabling it can't reintroduce the writer-exit deadlock.)
			select {
			case c.wch <- &Op{
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
			}:
			case <-ctx.Done():
				return
			}
			time.Sleep(time.Minute)
		}
	}
}

// requestGuildMembersExternal is the management-RPC path (gRPC). Unlike the
// internal default-drop variant it blocks on the send, aborting only if the
// connection is torn down. Callers (RequestGuildMembers) gate on an authed
// conn; the ctx check here closes the residual window where the conn dies
// between that gate and the send, so the op is dropped + logged rather than
// orphaned into a wch the dead writer will never drain.
func (c *conn) requestGuildMembersExternal(guildID int64) {
	if c.ctx.Err() != nil {
		c.s.log.Info(c.ctx, "drop members request: connection closed", slog.F("guild", guildID))
		return
	}

	op := &Op{
		Op: 8,
		D: RequestGuildMembers{
			GuildID: guildID,
		},
	}

	c.s.log.Info(c.ctx, "sending members request", slog.F("guild", guildID))
	select {
	case c.wch <- op:
	case <-c.ctx.Done():
	}
}
