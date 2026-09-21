package gatewayws

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
)

// newSilentEtcdClient points an etcd client at a listener that accepts TCP
// connections but never speaks gRPC. Grant would wait forever without a caller
// deadline because clientv3 retries it with WaitForReady.
func newSilentEtcdClient(t *testing.T) *clientv3.Client {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-release
				_ = conn.Close()
			}()
		}
	}()

	t.Cleanup(func() {
		close(release)
		_ = ln.Close()
		<-acceptDone
	})

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://" + ln.Addr().String()},
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestInitEtcdLeaseGrantIsBoundedWhenServerNeverAnswers(t *testing.T) {
	s, c := newResumableSession(t, &shardInfoRecorder{})
	s.etcd = newSilentEtcdClient(t)

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- c.initEtcdWithin(200 * time.Millisecond)
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("etcd lease grant did not return: an unbounded Grant parks the reconnect loop forever")
	}

	if err == nil {
		t.Fatal("lease grant to a server that never answers returned no error")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("lease grant took %v, want it bounded by the supplied deadline", took)
	}
	if !strings.Contains(err.Error(), "grant etcd lease") {
		t.Fatalf("initEtcdWithin() error = %v, want a lease-grant error", err)
	}
	if c.ctx.Err() != nil {
		t.Fatalf("connection context is %v: a lease deadline must not cancel the connection context", c.ctx.Err())
	}
	if c.etcdSess != nil || c.identifyMu != nil {
		t.Fatal("failed lease grant created an etcd session or identify mutex")
	}
}

func TestNonResumableConnCannotReachDiscordDialWithoutIdentifyLock(t *testing.T) {
	s, oldConn := newResumableSession(t, &shardInfoRecorder{})
	oldConn.cancel()
	atomic.StoreInt64(&s.seq, 0)
	s.sessID = ""
	s.resumeURL = ""
	s.etcd = newSilentEtcdClient(t)

	if s.shouldResume() {
		t.Fatal("test setup must start with a non-resumable session")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)
	c := s.newConn(ctx, cancel)

	err := c.run(context.Background())
	if err == nil {
		t.Fatal("non-resumable connection proceeded without an identify lock")
	}
	if strings.Contains(err.Error(), "dial gateway") {
		t.Fatalf("non-resumable connection reached the Discord dial without an identify lock: %v", err)
	}
	if !strings.Contains(err.Error(), "grant etcd lease") {
		t.Fatalf("non-resumable connection failed at %v, want etcd lease setup before any Discord dial", err)
	}
	if c.identifyMu != nil {
		t.Fatal("connection reports an identify mutex even though etcd never granted its lease")
	}
}
