package gatewayws

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/tatsuworks/gateway/internal/state"
)

type shardInfoRecorder struct {
	state.DB

	mu        sync.Mutex
	calls     int
	shard     int
	name      string
	seq       int64
	sess      string
	resumeURL string
}

func (r *shardInfoRecorder) SetShardInfo(ctx context.Context, shard int, name string, seq int64, sess string, resumeURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.shard = shard
	r.name = name
	r.seq = seq
	r.sess = sess
	r.resumeURL = resumeURL
	return nil
}

func newResumableSession(t *testing.T, db state.DB) (*Session, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		ctx:       ctx,
		cancel:    cancel,
		log:       slogtest.Make(t, nil),
		stateDB:   db,
		shardID:   42,
		name:      "gateway",
		seq:       12345,
		sessID:    "existing-session",
		resumeURL: "wss://resume.example",
	}
	if !s.shouldResume() {
		t.Fatal("test setup must start with a resumable session")
	}
	return s, cancel
}

// ForceIdentify only signals: it sets the flag and cancels the active
// connection. It must NOT touch the read-loop-owned resume fields itself (that
// is applyForceIdentify's job, on the read-loop goroutine), and it must NOT
// persist directly — persistShardInfo is flag-aware instead.
func TestForceIdentifySignalsWithoutMutatingResumeState(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	s.ForceIdentify()

	if atomic.LoadInt32(&s.forceIdentify) != 1 {
		t.Fatal("ForceIdentify did not set the forceIdentify flag")
	}
	// In-memory resume tuple is untouched until the read loop applies it.
	if !s.shouldResume() {
		t.Fatal("ForceIdentify cleared resume state on the caller goroutine; that must happen in the read loop")
	}
	if db.calls != 0 {
		t.Fatalf("ForceIdentify persisted directly (calls=%d); persistence must go through the read loop", db.calls)
	}
	select {
	case <-s.ctx.Done():
	default:
		t.Fatal("ForceIdentify did not cancel the active shard context")
	}
}

// While the flag is pending, persistShardInfo must write a cleared tuple even
// though the in-memory fields still hold the old resume values. This covers the
// cancelled connection's own deferred persist on shutdown.
func TestPersistShardInfoClearsWhileForcePending(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	atomic.StoreInt32(&s.forceIdentify, 1)
	s.persistShardInfo()

	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d, want 1", db.calls)
	}
	if db.seq != 0 || db.sess != "" || db.resumeURL != "" {
		t.Fatalf("persisted resume tuple while force pending = seq:%d sess:%q resumeURL:%q, want cleared",
			db.seq, db.sess, db.resumeURL)
	}
}

// applyForceIdentify runs on the read-loop goroutine: it consumes the flag,
// clears the resume tuple, and persists the cleared state.
func TestApplyForceIdentifyClearsConsumesAndPersists(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	atomic.StoreInt32(&s.forceIdentify, 1)
	s.applyForceIdentify()

	if atomic.LoadInt32(&s.forceIdentify) != 0 {
		t.Fatal("applyForceIdentify did not consume the flag")
	}
	if s.shouldResume() {
		t.Fatal("applyForceIdentify left the session resumable")
	}
	if got := atomic.LoadInt64(&s.seq); got != 0 {
		t.Fatalf("seq = %d, want 0", got)
	}
	if s.sessID != "" || s.resumeURL != "" {
		t.Fatalf("sessID=%q resumeURL=%q, want both empty", s.sessID, s.resumeURL)
	}
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d, want 1", db.calls)
	}
	if db.shard != 42 || db.name != "gateway" || db.seq != 0 || db.sess != "" || db.resumeURL != "" {
		t.Fatalf("persisted shard info = shard:%d name:%q seq:%d sess:%q resumeURL:%q, want cleared shard 42",
			db.shard, db.name, db.seq, db.sess, db.resumeURL)
	}

	// A second apply with no flag set is a no-op.
	s.applyForceIdentify()
	if db.calls != 1 {
		t.Fatalf("applyForceIdentify persisted with no flag set (calls=%d)", db.calls)
	}
}

// ForceIdentify must be safe to call from a foreign goroutine while the
// read-loop goroutine concurrently touches seq/sessID/resumeURL through the
// same paths Open uses (shouldResume, persistShardInfo, applyForceIdentify).
// Run with -race to catch regressions of the original data race.
func TestForceIdentifyConcurrentWithReadLoop(t *testing.T) {
	db := &shardInfoRecorder{}
	s, _ := newResumableSession(t, db)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Stand in for the read-loop goroutine: it owns the resume fields.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s.applyForceIdentify()
			_ = s.shouldResume()
			s.persistShardInfo()
			atomic.StoreInt64(&s.seq, 7)
		}
	}()

	// Stand in for the gRPC handler goroutine firing repeated force-identifies.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			s.ForceIdentify()
		}
		close(stop)
	}()

	wg.Wait()
}
