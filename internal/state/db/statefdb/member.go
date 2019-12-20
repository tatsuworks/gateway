package statefdb

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetGuildMembers(guild int64, raws map[int64][]byte) error {
	return db.setGuildETFs(guild, raws, db.fmtGuildMemberKey)
}

func (db *DB) DeleteGuildMembers(guild int64) error {
	pre, _ := fdb.PrefixRange(db.fmtGuildMemberPrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(pre)
		return nil
	})
}

func (db *DB) SetGuildMember(guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildMemberKey(guild, user), raw)
		return nil
	})
}

func (db *DB) GetGuildMember(guild, user int64) ([]byte, error) {
	var m []byte

	err := db.Transact(func(t fdb.Transaction) error {
		m = t.Get(db.fmtGuildMemberKey(guild, user)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (db *DB) GetGuildMembers(guild int64) ([]fdb.KeyValue, error) {
	var (
		raws   []fdb.KeyValue
		pre, _ = fdb.PrefixRange(db.fmtGuildMemberPrefix(guild))
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

func (db *DB) DeleteGuildMember(guild, user int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildMemberKey(guild, user))
		return nil
	})
}
