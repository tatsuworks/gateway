package statefdb

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
)

func (db *DB) GetShardInfo(ctx context.Context, shard int, name string) (sess string, seq int64, err error) {
	err = db.Transact(func(t fdb.Transaction) error {
		sess = string(t.Get(db.fmtShardSessKey(shard, name)).MustGet())
		val := t.Get(db.fmtShardSeqKey(shard, name)).MustGet()
		if val != nil {
			seq = bytesToInt64(val)
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	return sess, seq, nil
}
func (db *DB) SetSequence(ctx context.Context, shard int, name string, seq int64) error {
	err := db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtShardSeqKey(shard, name), int64ToBytes(seq))
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
func (db *DB) GetSequence(ctx context.Context, shard int, name string) (int64, error) {
	var seq int64
	err := db.Transact(func(t fdb.Transaction) error {
		val := t.Get(db.fmtShardSeqKey(shard, name)).MustGet()
		if val != nil {
			seq = bytesToInt64(val)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return seq, nil
}
func (db *DB) SetSessionID(ctx context.Context, shard int, name, sess string) error {
	err := db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtShardSessKey(shard, name), []byte(sess))
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
func (db *DB) GetSessionID(ctx context.Context, shard int, name string) (string, error) {
	var sid string
	err := db.Transact(func(t fdb.Transaction) error {
		sid = string(t.Get(db.fmtShardSessKey(shard, name)).MustGet())
		return nil
	})
	if err != nil {
		return "", err
	}
	return sid, nil
}
