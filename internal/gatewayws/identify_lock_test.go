package gatewayws

import (
	"context"
	"strings"
	"testing"

	"github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/etcd-io/etcd/clientv3/concurrency"

	"github.com/tatsuworks/gateway/discord"
)

// A RESUMEing connection never takes the cross-shard identify lock, so since
// the etcd session moved onto the IDENTIFY branch of conn.run it has no etcd
// session and no mutex at all.
//
// INVALID_SESSION is the ordinary way a resume fails -- a session Discord has
// already expired draws op 9 rather than RESUMED -- and it is handled on
// exactly that mutex-less connection. Dereferencing the mutex there panics on
// the read-loop goroutine, and an unrecovered panic in a goroutine takes the
// process with it: all 64 shards on the pod, on the single most common resume
// failure. This pins the guard that prevents it.
func TestInvalidSessionOnResumingConnDoesNotTouchEtcd(t *testing.T) {
	db := &shardInfoRecorder{}
	s, c := newResumableSession(t, db)

	if c.identifyMu != nil || c.etcdSess != nil {
		t.Fatal("test setup: a resuming conn must hold neither an etcd session nor an identify mutex")
	}

	handled, err := c.handleInternalEvent(&discord.Event{Op: 9})

	if !handled {
		t.Fatal("op 9 was not handled internally")
	}
	if err == nil {
		t.Fatal("op 9 must tear the connection down with an error so the next connect IDENTIFYs")
	}

	// The tuple is discarded in memory and persisted cleared, so neither this
	// process nor a later pod restart retries the dead session.
	if s.shouldResume() {
		t.Fatal("op 9 did not discard the resume tuple")
	}
	if db.calls == 0 {
		t.Fatal("op 9 did not persist the cleared resume tuple")
	}
	if db.sess != "" || db.resumeURL != "" || db.seq != 0 {
		t.Fatalf("op 9 persisted a still-resumable row: seq=%d sess=%q resume_url=%q", db.seq, db.sess, db.resumeURL)
	}
}

// The point of the whole change: a shard holding a resume tuple must be able to
// connect while etcd is unavailable, because the single-replica etcd-gateway pod
// is evicted by any node drain and the gateway pods drained alongside it are the
// ones reconnecting right then.
//
// s.etcd is left nil, so any call into initEtcd on this path nil-panics rather
// than failing quietly — the assertion is that run() reaches the dial without
// having touched etcd. The dial is aimed at a closed local port so it is
// refused immediately, with no DNS and no outbound traffic.
func TestResumingConnReachesTheDialWithoutEtcd(t *testing.T) {
	s, c := newResumableSession(t, &shardInfoRecorder{})
	s.etcd = nil
	s.resumeURL = "ws://127.0.0.1:1"

	if !s.shouldResume() {
		t.Fatal("test setup must start with a resumable session")
	}

	err := c.run(context.Background())

	if err == nil {
		t.Fatal("expected the dial against a closed port to fail")
	}
	// Reaching the dial at all is the assertion: before this change run() called
	// initEtcd first and would have panicked on the nil client above.
	if !strings.Contains(err.Error(), "dial gateway") {
		t.Fatalf("resume path failed before the dial, so it did not skip etcd: %v", err)
	}
	if c.identifyMu != nil || c.etcdSess != nil {
		t.Fatal("resume path created an etcd session")
	}
}

// releaseIdentifyLockIfHeld must be a no-op for a connection that never took
// the lock, and must not reach etcd for one: the mutex below has a nil session,
// so a call through to etcd would nil-panic, and a nil mutex would panic
// sooner still.
func TestReleaseIdentifyLockIfHeldIsNoOpWhenNotHeld(t *testing.T) {
	for name, mu := range map[string]*concurrency.Mutex{
		"nil mutex (resuming conn, no etcd session)": nil,
		"never locked": concurrency.NewMutex(nil, IdentifyMutexRootName+"0"),
	} {
		t.Run(name, func(t *testing.T) {
			_, c := newResumableSession(t, &shardInfoRecorder{})
			c.identifyMu = mu

			if err := c.releaseIdentifyLockIfHeld(); err != nil {
				t.Fatalf("releasing an unheld lock returned an error: %v", err)
			}
		})
	}
}

// The two sentinel keys a Mutex carries when it holds nothing. "" is what
// concurrency.NewMutex initialises myKey to; "\x00" is what Mutex.Unlock leaves
// behind once the lock has been dropped, which is the state op 9 finds when it
// arrives after the post-READY release goroutine has already run. Unlocking on
// either issues a pointless Delete against etcd.
func TestIdentifyLockHeldRejectsBothUnheldSentinels(t *testing.T) {
	for key, want := range map[string]bool{
		"":                        false,
		"\x00":                    false,
		"/gateway/identify/0/1f4": true,
	} {
		if got := identifyLockHeld(key); got != want {
			t.Errorf("identifyLockHeld(%q) = %v, want %v", key, got, want)
		}
	}
}

// Mutex.IsOwner is not an ownership test and must never be used as one. It
// builds a clientv3.Cmp to be evaluated *inside* a Txn, and clientv3.Compare
// sets .Result straight from the operator string -- so comparing .Result to
// Compare_EQUAL is true for every mutex, held or not.
//
// This is pinned rather than merely deleted because the construct reads exactly
// like the check it is not, and it guarded the INVALID_SESSION release path for
// as long as that path existed.
func TestIsOwnerIsNotAnOwnershipCheck(t *testing.T) {
	mu := concurrency.NewMutex(nil, IdentifyMutexRootName+"0")

	if mu.Key() != "" {
		t.Fatalf("a never-locked mutex should hold no key, got %q", mu.Key())
	}
	if mu.IsOwner().Result != etcdserverpb.Compare_EQUAL {
		t.Skip("clientv3.Compare no longer sets Result from the operator; re-check the release guard")
	}

	t.Log("confirmed: IsOwner().Result == Compare_EQUAL for a mutex owning nothing; " +
		"ownership must be decided on Key(), as releaseIdentifyLockIfHeld does")
}
