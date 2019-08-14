package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetGuildRole(guild, role int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildRoleKey(guild, role), raw)
		return nil
	})
}

func (db *DB) SetGuildRoles(guild int64, roles map[int64][]byte) error {
	return db.setGuildETFs(guild, roles, db.fmtGuildRoleKey)
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
