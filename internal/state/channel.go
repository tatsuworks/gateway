package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetChannel(guild, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtChannelKey(id), raw)
		t.Set(db.fmtGuildChannelKey(guild, id), raw)
		return nil
	})
}

func (db *DB) DeleteChannel(guild, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtChannelKey(id))
		t.Clear(db.fmtGuildChannelKey(guild, id))
		return nil
	})
}

func (db *DB) SetVoiceState(guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildVoiceStateKey(guild, user), raw)
		return nil
	})
}
