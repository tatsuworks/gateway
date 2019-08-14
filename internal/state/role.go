package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetGuildRole(guild, role int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildRoleKey(guild, role), raw)
		return nil
	})
}

func (db *DB) GetGuildRole(guild, role int64) ([]byte, error) {
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

func (db *DB) SetGuildRoles(guild int64, roles map[int64][]byte) error {
	return db.setGuildETFs(guild, roles, db.fmtGuildRoleKey)
}

func (db *DB) GetGuildRoles(guild int64) ([]fdb.KeyValue, error) {
	var (
		raws   []fdb.KeyValue
		pre, _ = fdb.PrefixRange(db.fmtGuildRolePrefix(guild))
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return raws, nil
}

func (db *DB) DeleteGuildRoles(guild int64) error {
	pre, _ := fdb.PrefixRange(db.fmtGuildRolePrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(pre)
		return nil
	})
}

func (db *DB) DeleteGuildRole(guild, role int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildRoleKey(guild, role))
		return nil
	})
}
