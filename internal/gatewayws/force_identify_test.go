package gatewayws

import (
	"context"
	"sync/atomic"
	"testing"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/tatsuworks/gateway/internal/state"
)

type shardInfoRecorder struct {
	state.DB

	calls     int
	shard     int
	name      string
	seq       int64
	sess      string
	resumeURL string
}

func (r *shardInfoRecorder) SetShardInfo(ctx context.Context, shard int, name string, seq int64, sess string, resumeURL string) error {
	r.calls++
	r.shard = shard
	r.name = name
	r.seq = seq
	r.sess = sess
	r.resumeURL = resumeURL
	return nil
}

func TestForceIdentifyClearsResumeStatePersistsAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	db := &shardInfoRecorder{}
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

	s.ForceIdentify()

	if s.shouldResume() {
		t.Fatal("ForceIdentify left the session resumable")
	}
	if got := atomic.LoadInt64(&s.seq); got != 0 {
		t.Fatalf("seq = %d, want 0", got)
	}
	if s.sessID != "" {
		t.Fatalf("sessID = %q, want empty", s.sessID)
	}
	if s.resumeURL != "" {
		t.Fatalf("resumeURL = %q, want empty", s.resumeURL)
	}
	if db.calls != 1 {
		t.Fatalf("SetShardInfo calls = %d, want 1", db.calls)
	}
	if db.shard != 42 || db.name != "gateway" || db.seq != 0 || db.sess != "" || db.resumeURL != "" {
		t.Fatalf("persisted shard info = shard:%d name:%q seq:%d sess:%q resumeURL:%q, want cleared shard 42",
			db.shard, db.name, db.seq, db.sess, db.resumeURL)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("ForceIdentify did not cancel the active shard context")
	}
}
