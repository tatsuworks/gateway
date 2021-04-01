package statefdb

import (
	"encoding/binary"

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

func (db *DB) setGuildETFs(guild int64, etfs map[int64][]byte, keySetter func(t fdb.Transaction, guild, id int64, e []byte)) error {
	eg := new(errgroup.Group)

	send := func(guild int64, etfs map[int64][]byte, keySetter func(t fdb.Transaction, guild, id int64, e []byte)) {
		eg.Go(func() error {
			return db.Transact(func(t fdb.Transaction) error {
				opts := t.Options()
				opts.SetReadYourWritesDisable()
				opts.SetPrioritySystemImmediate()

				for id, e := range etfs {
					opts.SetNextWriteNoWriteConflictRange()
					keySetter(t, guild, id, e)
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
				send(guild, bufMap, keySetter)
				bufMap = make(map[int64][]byte, maxPerTxn)
			}
		}
	}

	send(guild, bufMap, keySetter)
	return eg.Wait()
}

func int64ToBytes(i int64) []byte {
	var b [8]byte
	// it is OK to use PutUint64 because it doesn't break negative numbers
	// we just need to do int64(...) when retrieving it back
	binary.LittleEndian.PutUint64(b[:], uint64(i))
	return b[:]
}

func bytesToInt64(b []byte) int64 {
	return int64(bytesToUint64(b))
}

func bytesToUint64(b []byte) uint64 {
	if len(b) != 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}
