package gatewayws

import (
	"context"
	"testing"
	"time"

	"cdr.dev/slog"
)

func TestDivergenceTiming_RecordIncrementsCounters(t *testing.T) {
	dt := NewDivergenceTiming(slog.Logger{}, 100*time.Millisecond, 1000)
	dt.record(context.Background(), 5*time.Millisecond)
	dt.record(context.Background(), 200*time.Millisecond)
	dt.record(context.Background(), 50*time.Millisecond)

	dt.mu.Lock()
	defer dt.mu.Unlock()
	if dt.count != 3 {
		t.Fatalf("count: got %d want 3", dt.count)
	}
	if dt.slowCount != 1 {
		t.Fatalf("slowCount: got %d want 1 (only the 200ms call)", dt.slowCount)
	}
	if dt.maxNanos != int64(200*time.Millisecond) {
		t.Fatalf("maxNanos: got %d want %d", dt.maxNanos, int64(200*time.Millisecond))
	}
}

func TestDivergenceTiming_NilSafe(t *testing.T) {
	var dt *DivergenceTiming
	dt.record(context.Background(), 10*time.Millisecond) // must not panic
}

func TestDivergenceTiming_NoSlowThreshold(t *testing.T) {
	dt := NewDivergenceTiming(slog.Logger{}, 0, 1000)
	dt.record(context.Background(), 500*time.Millisecond)
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if dt.slowCount != 0 {
		t.Fatalf("slow threshold 0 should disable slow tracking; got %d", dt.slowCount)
	}
}
