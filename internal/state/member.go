package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetGuildMembers(guild int64, raws map[int64][]byte) error {
	return db.setGuildETFs(guild, raws, db.fmtGuildMemberKey)
}

func (db *DB) SetGuildMember(guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildMemberKey(guild, user), raw)
		return nil
	})
}

func (db *DB) DeleteGuildMember(guild, user int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildMemberKey(guild, user))
		return nil
	})
}
