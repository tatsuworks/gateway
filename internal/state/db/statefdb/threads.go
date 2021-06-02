package statefdb

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"golang.org/x/xerrors"
)

func (db *DB) SetThread(_ context.Context, guild, channel, owner, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtThreadKey(id), raw)
		t.Set(db.fmtGuildChannelThreadKey(guild, channel, owner, id), raw)
		return nil
	})
}

func (db *DB) GetThread(_ context.Context, id int64) ([]byte, error) {
	var c []byte

	err := db.Transact(func(t fdb.Transaction) error {
		c = t.Get(db.fmtThreadKey(id)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (db *DB) GetThreadsCount(_ context.Context) (int, error) {
	rr, _ := fdb.PrefixRange(db.fmtThreadPrefix())
	return db.keyCountForPrefix(rr)
}

func (db *DB) GetThreads(_ context.Context) ([][]byte, error) {
	var (
		raws   []fdb.KeyValue
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtThreadPrefix())
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("read threads: %w", err)
	}

	out = make([][]byte, len(raws))
	for i, e := range raws {
		out[i] = e.Value
	}

	return out, err
}

func (db *DB) GetGuildThreads(_ context.Context, guild int64) ([][]byte, error) {
	var (
		raws   []fdb.KeyValue
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtGuildThreadPrefix(guild))
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("read threads: %w", err)
	}

	out = make([][]byte, len(raws))
	for i, e := range raws {
		out[i] = e.Value
	}

	return out, err
}

func (db *DB) DeleteThread(_ context.Context, guild, id int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtThreadKey(id))
		t.Clear(db.fmtGuildThreadKey(guild, id))
		return nil
	})
}

func (db *DB) SetThreads(_ context.Context, guild int64, threads map[int64][]byte) error {
	err := db.setETFs(threads, db.fmtThreadKey)
	if err != nil {
		return xerrors.Errorf("set threads: %w", err)
	}

	err = db.setGuildETFs(guild, threads, db.fmtGuildThreadKey)
	if err != nil {
		return xerrors.Errorf("set guild threads: %w", err)
	}

	return nil
}

// this will leak threads in the main pool.
// TODO: fix
func (db *DB) DeleteThreads(_ context.Context, guild int64) error {
	gRange, _ := fdb.PrefixRange(db.fmtGuildThreadPrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(gRange)
		return nil
	})
}
