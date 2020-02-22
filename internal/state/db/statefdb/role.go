package statefdb

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
)

func (db *DB) SetGuildRole(_ context.Context, guild, role int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildRoleKey(guild, role), raw)
		return nil
	})
}

func (db *DB) GetGuildRole(_ context.Context, guild, role int64) ([]byte, error) {
	var r []byte

	err := db.Transact(func(t fdb.Transaction) error {
		r = t.Get(db.fmtGuildRoleKey(guild, role)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (db *DB) SetGuildRoles(_ context.Context, guild int64, roles map[int64][]byte) error {
	return db.setGuildETFs(guild, roles, db.fmtGuildRoleKey)
}

func (db *DB) GetGuildRoles(_ context.Context, guild int64) ([][]byte, error) {
	var (
		raws   []fdb.KeyValue
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtGuildRolePrefix(guild))
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, err
	}

	out = make([][]byte, len(raws))
	for i, e := range raws {
		out[i] = e.Value
	}

	return out, err
}

func (db *DB) DeleteGuildRoles(_ context.Context, guild int64) error {
	pre, _ := fdb.PrefixRange(db.fmtGuildRolePrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(pre)
		return nil
	})
}

func (db *DB) DeleteGuildRole(_ context.Context, guild, role int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildRoleKey(guild, role))
		return nil
	})
}
