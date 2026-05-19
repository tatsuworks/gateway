package gatewayws

import (
	"context"
	"sync"
	"time"

	"cdr.dev/slog"
)

// DivergenceTiming records per-call durations for the divergence
// check's DB read. Shared across shards in a process so summary logs
// fire at a manageable rate instead of once per shard.
type DivergenceTiming struct {
	log          slog.Logger
	slowThresh   time.Duration
	summaryEvery int64

	mu        sync.Mutex
	count     int64
	sumNanos  int64
	maxNanos  int64
	slowCount int64
}

func NewDivergenceTiming(log slog.Logger, slowThresh time.Duration, summaryEvery int64) *DivergenceTiming {
	if summaryEvery <= 0 {
		summaryEvery = 1000
	}
	return &DivergenceTiming{
		log:          log,
		slowThresh:   slowThresh,
		summaryEvery: summaryEvery,
	}
}

func (t *DivergenceTiming) record(ctx context.Context, dur time.Duration) {
	if t == nil {
		return
	}

	t.mu.Lock()
	t.count++
	t.sumNanos += int64(dur)
	if int64(dur) > t.maxNanos {
		t.maxNanos = int64(dur)
	}
	slow := t.slowThresh > 0 && dur >= t.slowThresh
	if slow {
		t.slowCount++
	}
	emitSummary := t.count%t.summaryEvery == 0

	var (
		summaryCount, summaryMaxN, summarySlow int64
		summaryMean                            time.Duration
	)
	if emitSummary {
		summaryCount = t.count
		summaryMaxN = t.maxNanos
		summarySlow = t.slowCount
		summaryMean = time.Duration(t.sumNanos / t.count)
	}
	t.mu.Unlock()

	if slow {
		t.log.Warn(ctx, "divergence DB call slow",
			slog.F("duration", dur.String()),
			slog.F("threshold", t.slowThresh.String()))
	}
	if emitSummary {
		t.log.Info(ctx, "divergence DB timing summary",
			slog.F("count", summaryCount),
			slog.F("mean", summaryMean.String()),
			slog.F("max", time.Duration(summaryMaxN).String()),
			slog.F("slow_count", summarySlow),
			slog.F("slow_threshold", t.slowThresh.String()))
	}
}
