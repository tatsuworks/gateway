package statefdb

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetChannelMessage(channel, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtChannelMessageKey(channel, id), raw)
		return nil
	})
}

func (db *DB) GetChannelMessage(channel, id int64) ([]byte, error) {
	var m []byte

	err := db.Transact(func(t fdb.Transaction) error {
		m = t.Get(db.fmtChannelMessageKey(channel, id)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return m, err
}

func (db *DB) DeleteChannelMessage(channel, id int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtChannelMessageKey(channel, id))
		return nil
	})
}

func (db *DB) SetChannelMessageReaction(channel, id, user int64, name interface{}, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtMessageReactionKey(channel, id, user, name), raw)
		return nil
	})
}

func (db *DB) DeleteChannelMessageReaction(channel, id, user int64, name interface{}) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtMessageReactionKey(channel, id, user, name))
		return nil
	})
}

func (db *DB) DeleteChannelMessageReactions(channel, id, user int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		pre, _ := fdb.PrefixRange(db.fmtMessageReactionPrefix(channel, id, user))

		t.ClearRange(pre)
		return nil
	})
}
