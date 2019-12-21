package statefdb

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
)

func (db *DB) SetGuild(ctx context.Context, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildKey(id), raw)
		return nil
	})
}

func (db *DB) GetGuild(ctx context.Context, id int64) ([]byte, error) {
	var g []byte

	err := db.Transact(func(t fdb.Transaction) error {
		g = t.Get(db.fmtGuildKey(id)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return g, nil
}

func (db *DB) GetGuildCount(ctx context.Context) (int, error) {
	rr, _ := fdb.PrefixRange(db.fmtGuildPrefix())
	return db.keyCountForPrefix(rr)
}

func (db *DB) DeleteGuild(ctx context.Context, id int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildKey(id))
		return nil
	})
}

func (db *DB) SetGuildBan(ctx context.Context, guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildBanKey(guild, user), raw)
		return nil
	})
}

func (db *DB) GetGuildBan(ctx context.Context, guild, user int64) ([]byte, error) {
	var gb []byte

	err := db.Transact(func(t fdb.Transaction) error {
		gb = t.Get(db.fmtGuildBanKey(guild, user)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return gb, nil
}

func (db *DB) DeleteGuildBan(ctx context.Context, guild, user int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildBanKey(guild, user))
		return nil
	})
}
