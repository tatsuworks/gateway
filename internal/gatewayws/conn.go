package gatewayws

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/etcd-io/etcd/clientv3/concurrency"
	"nhooyr.io/websocket"
)

// conn is a single gateway connection. A fresh conn is built per Open and
// discarded when that connection dies, so connection-scoped state (channels,
// socket, heartbeat timing) can never leak across reconnects.
type conn struct {
	s *Session

	ctx    context.Context
	cancel context.CancelFunc

	wsConn *websocket.Conn
	zr     io.ReadCloser

	interval time.Duration
	trace    string

	wch    chan *Op
	prioch chan *Op

	buf *bytes.Buffer

	etcdSess   *concurrency.Session
	identifyMu *concurrency.Mutex

	authed atomic.Bool

	// mu guards the timing/state fields below, which are read by management
	// calls (Status/LongLastAck) on the manager goroutine while the read-loop
	// and heartbeat goroutines write them. Accessors are added in Task 4.
	mu      sync.Mutex
	lastHB  time.Time
	lastAck time.Time
	ready   time.Time
	// disconnected is when the websocket read loop stopped, stamped before run's
	// deferred teardown. See readyFor.
	disconnected time.Time
	curState     string
}

func (s *Session) newConn(ctx context.Context, cancel context.CancelFunc) *conn {
	return &conn{
		s:      s,
		ctx:    ctx,
		cancel: cancel,
		wch:    make(chan *Op, 2000),
		prioch: make(chan *Op),
	}
}

// The accessors below guard the timing/state fields: the read-loop and
// heartbeat goroutines write them while Status/LongLastAck (manager goroutine)
// read them.

func (c *conn) setState(state string) {
	c.mu.Lock()
	c.curState = state
	c.mu.Unlock()
}

func (c *conn) markHB(t time.Time) {
	c.mu.Lock()
	c.lastHB = t
	c.mu.Unlock()
}

func (c *conn) markAck(t time.Time) {
	c.mu.Lock()
	c.lastAck = t
	c.mu.Unlock()
}

func (c *conn) markReady(t time.Time) {
	c.mu.Lock()
	c.ready = t
	c.mu.Unlock()
}

// markDisconnected records when the websocket read loop stopped. It is stamped
// where the loop exits rather than where run returns, because run's deferred
// teardown is unbounded — persistShardInfo writes to Postgres on
// context.Background() with no deadline. See readyFor.
func (c *conn) markDisconnected(t time.Time) {
	c.mu.Lock()
	c.disconnected = t
	c.mu.Unlock()
}

// readyFor returns how long this connection had been authenticated as of now,
// or 0 if it never got there. It is the connection-health measure the reconnect
// backoff scores an attempt on: Session.Open also covers etcd setup and
// acquireIdentifyLock (up to 160s), so the duration of the attempt as a whole
// says nothing about whether a usable connection was ever established.
func (c *conn) readyFor(now time.Time) time.Duration {
	c.mu.Lock()
	ready, disconnected := c.ready, c.disconnected
	c.mu.Unlock()

	if ready.IsZero() {
		return 0
	}
	// Uptime ends at the disconnect, not at the caller's clock. Open asks for
	// this after run returns, and run's teardown can outlast the connection by
	// an unbounded amount: it defers persistShardInfo, which writes to Postgres
	// on context.Background() with no deadline. Counting that stall as uptime
	// would let a connection that lasted seconds clear reconnectHealthyUptime
	// and reset the manager's backoff ladder during a database outage — exactly
	// when it should be escalating — and would report the stall as connected_for
	// on a log line written to be read during an incident.
	if !disconnected.IsZero() {
		now = disconnected
	}
	return now.Sub(ready)
}

// snapshot returns the guarded state fields in one locked read.
func (c *conn) snapshot() (curState string, lastHB, lastAck, ready time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curState, c.lastHB, c.lastAck, c.ready
}
