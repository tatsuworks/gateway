package state

import "github.com/apple/foundationdb/bindings/go/src/fdb"

func (db *DB) SetChannelMessage(channel, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtChannelMessageKey(channel, id), raw)
		return nil
	})
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
