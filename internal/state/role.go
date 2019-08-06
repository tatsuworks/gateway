package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetGuildRole(guild, role int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildRoleKey(guild, role), raw)
		return nil
	})
}

func (db *DB) DeleteGuildRole(guild, role int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildRoleKey(guild, role))
		return nil
	})
}
