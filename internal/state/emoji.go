package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetGuildEmojis(guild int64, raws map[int64][]byte) error {
	return db.setGuildETFs(guild, raws, db.fmtGuildEmojiKey)
}

func (db *DB) DeleteGuildEmojis(guild int64) error {
	pre, _ := fdb.PrefixRange(db.fmtGuildEmojiPrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(pre)
		return nil
	})
}

func (db *DB) SetGuildEmoji(guild, emoji int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildEmojiKey(guild, emoji), raw)
		return nil
	})
}

func (db *DB) GetGuildEmoji(guild, emoji int64) ([]byte, error) {
	var e []byte

	err := db.Transact(func(t fdb.Transaction) error {
		e = t.Get(db.fmtGuildEmojiKey(guild, emoji)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (db *DB) GetGuildEmojis(guild int64) ([]fdb.KeyValue, error) {
	var (
		raws   []fdb.KeyValue
		pre, _ = fdb.PrefixRange(db.fmtGuildEmojiPrefix(guild))
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

func (db *DB) DeleteGuildEmoji(guild, emoji int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildEmojiKey(guild, emoji))
		return nil
	})
}
