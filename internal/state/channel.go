package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetChannel(guild, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtChannelKey(id), raw)
		t.Set(db.fmtGuildChannelKey(guild, id), raw)
		return nil
	})
}

func (db *DB) GetChannel(id int64, raw []byte) ([]byte, error) {
	var c []byte

	err := db.Transact(func(t fdb.Transaction) error {
		c = t.Get(db.fmtChannelKey(id)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (db *DB) DeleteChannel(guild, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtChannelKey(id))
		t.Clear(db.fmtGuildChannelKey(guild, id))
		return nil
	})
}

func (db *DB) SetChannels(guild int64, channels map[int64][]byte) error {
	return db.setETFs(channels, db.fmtChannelKey)
}

// this will leak channels in the main pool.
// TODO: fix
func (db *DB) DeleteChannels(guild int64) error {
	gRange, _ := fdb.PrefixRange(db.fmtGuildChannelPrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(gRange)
		return nil
	})
}

func (db *DB) SetVoiceState(guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildVoiceStateKey(guild, user), raw)
		return nil
	})
}
