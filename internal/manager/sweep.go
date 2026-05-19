package manager

import (
	"context"
	"errors"
	"strconv"
	"time"

	"cdr.dev/slog"
	"github.com/coreos/etcd/clientv3"
	"golang.org/x/time/rate"
)

// sweepCursorKey is the etcd prefix under which each manager stores its
// sweep cursor position. The full key is keyed by name AND shard range
// so multiple gateway processes sharing a name (different shard ranges,
// same deployment) maintain independent cursors and don't race on
// writes or end up redundantly scanning guilds outside their range.
const sweepCursorKey = "/gateway/sweep_cursor/"

func (m *Manager) sweepCursorEtcdKey() string {
	return sweepCursorKey + m.name + "/" +
		strconv.Itoa(m.shardStart) + "-" + strconv.Itoa(m.shardStop)
}

// runGuildBackfillSweep is the bounded-rate background re-sync. It
// pages through all guilds in id-order, dispatches Request Guild
// Members to the owning shard for each id within this process's range,
// and persists the cursor so restarts don't reset progress.
//
// Designed to never load more than sweepBatch ids into memory at once.
func (m *Manager) runGuildBackfillSweep(start, stop int) {
	ctx := m.ctx

	if !m.sweepEnabled {
		m.log.Info(ctx, "guild backfill sweep disabled")
		return
	}
	if m.sweepRequestsPerSec <= 0 || m.sweepBatch <= 0 {
		m.log.Info(ctx, "guild backfill sweep config invalid, disabling",
			slog.F("rps", m.sweepRequestsPerSec),
			slog.F("batch", m.sweepBatch))
		return
	}

	cursor := m.loadSweepCursor()
	limiter := rate.NewLimiter(rate.Limit(m.sweepRequestsPerSec), max(1, m.sweepRequestsPerSec))

	m.log.Info(ctx, "guild backfill sweep started",
		slog.F("start_cursor", cursor),
		slog.F("rps", m.sweepRequestsPerSec),
		slog.F("batch", m.sweepBatch),
		slog.F("shard_range", []int{start, stop}))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		ids, err := m.db.GetGuildIDsAfter(batchCtx, cursor, m.sweepBatch)
		cancel()
		if err != nil {
			m.log.Error(ctx, "sweep query failed, backing off",
				slog.F("cursor", cursor), slog.Error(err))
			if !sleepCtx(ctx, 30*time.Second) {
				return
			}
			continue
		}

		if len(ids) == 0 {
			// End of data; wrap and pause briefly so an empty database
			// doesn't spin.
			m.log.Info(ctx, "sweep wrapped to start", slog.F("prior_cursor", cursor))
			cursor = 0
			m.persistSweepCursor(cursor)
			if !sleepCtx(ctx, 1*time.Minute) {
				return
			}
			continue
		}

		dispatched := 0
		skippedOutOfRange := 0
		skippedOffline := 0
		for _, gid := range ids {
			ownerShard := int((uint64(gid) >> 22) % uint64(m.shardCount))
			if ownerShard < start || ownerShard >= stop {
				skippedOutOfRange++
				continue
			}

			if err := limiter.Wait(ctx); err != nil {
				return
			}

			m.shardMu.Lock()
			sess := m.shards[ownerShard]
			m.shardMu.Unlock()
			if sess == nil {
				skippedOffline++
				continue
			}

			sess.RequestGuildMembers(gid)
			dispatched++
		}

		cursor = ids[len(ids)-1]
		m.persistSweepCursor(cursor)

		m.log.Debug(ctx, "sweep batch dispatched",
			slog.F("cursor", cursor),
			slog.F("batch_size", len(ids)),
			slog.F("dispatched", dispatched),
			slog.F("skipped_out_of_range", skippedOutOfRange),
			slog.F("skipped_offline", skippedOffline))

		// If the batch was full there's likely more immediately; loop
		// without delay (rate limiter still paces dispatch). If short,
		// we're at end-of-data — handled by the empty branch next iter.
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (m *Manager) loadSweepCursor() int64 {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	resp, err := m.etcd.Get(ctx, m.sweepCursorEtcdKey())
	if err != nil {
		m.log.Warn(m.ctx, "load sweep cursor failed, starting from 0",
			slog.Error(err))
		return 0
	}
	if len(resp.Kvs) == 0 {
		return 0
	}
	cursor, err := strconv.ParseInt(string(resp.Kvs[0].Value), 10, 64)
	if err != nil {
		m.log.Warn(m.ctx, "parse sweep cursor failed, starting from 0",
			slog.F("raw", string(resp.Kvs[0].Value)), slog.Error(err))
		return 0
	}
	return cursor
}

func (m *Manager) persistSweepCursor(cursor int64) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()

	_, err := m.etcd.Put(ctx, m.sweepCursorEtcdKey(), strconv.FormatInt(cursor, 10))
	if err != nil && !errors.Is(err, context.Canceled) {
		m.log.Warn(m.ctx, "persist sweep cursor failed",
			slog.F("cursor", cursor), slog.Error(err))
	}
}

// Compile-time assertion that clientv3.Client is the etcd client we
// expect; keeps the import concrete even if all uses in this file are
// via Manager.etcd.
var _ = (*clientv3.Client)(nil)
