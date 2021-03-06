package statefdb

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"golang.org/x/sync/errgroup"
)

var (
	FDBRangeWantAll = fdb.RangeOptions{Mode: fdb.StreamingModeWantAll}
)

func (db *DB) setETFs(etfs map[int64][]byte, key func(id int64) fdb.Key) error {
	eg := new(errgroup.Group)

	send := func(etfs map[int64][]byte, key func(id int64) fdb.Key) {
		eg.Go(func() error {
			return db.Transact(func(t fdb.Transaction) error {
				opts := t.Options()
				opts.SetReadYourWritesDisable()
				opts.SetPrioritySystemImmediate()

				for id, e := range etfs {
					opts.SetNextWriteNoWriteConflictRange()
					t.Set(key(id), e)
				}

				return nil
			})
		})
	}

	bufMap := etfs

	// FDB recommends 10KB per transaction. If we limit transactions to
	// 100 keys each, we allow an average of 100 bytes per k/v pair.
	if len(etfs) > 100 {
		bufMap = make(map[int64][]byte, 100)

		for i, e := range etfs {
			bufMap[i] = e

			if len(bufMap) >= 100 {
				send(bufMap, key)
				bufMap = make(map[int64][]byte, 100)
			}
		}
	}

	send(bufMap, key)
	return eg.Wait()
}

func (db *DB) setGuildETFs(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) error {
	eg := new(errgroup.Group)

	send := func(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) {
		eg.Go(func() error {
			return db.Transact(func(t fdb.Transaction) error {
				opts := t.Options()
				opts.SetReadYourWritesDisable()
				opts.SetPrioritySystemImmediate()

				for id, e := range etfs {
					opts.SetNextWriteNoWriteConflictRange()
					t.Set(key(guild, id), e)
				}

				return nil
			})
		})
	}

	bufMap := etfs

	// FDB recommends 10KB per transaction. If we limit transactions to
	// 100 keys each, we allow an average of 100 bytes per k/v pair.
	const maxPerTxn = 100
	if len(etfs) > maxPerTxn {
		bufMap = make(map[int64][]byte, maxPerTxn)

		for i, e := range etfs {
			bufMap[i] = e

			if len(bufMap) >= maxPerTxn {
				send(guild, bufMap, key)
				bufMap = make(map[int64][]byte, maxPerTxn)
			}
		}
	}

	send(guild, bufMap, key)
	return eg.Wait()
}
