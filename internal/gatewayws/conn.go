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
	mu       sync.Mutex
	lastHB   time.Time
	lastAck  time.Time
	ready    time.Time
	curState string
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

// snapshot returns the guarded state fields in one locked read.
func (c *conn) snapshot() (curState string, lastHB, lastAck, ready time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curState, c.lastHB, c.lastAck, c.ready
}
