package statefdb

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
)

func (db *DB) SetGuild(_ context.Context, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildKey(id), raw)
		return nil
	})
}

func (db *DB) GetGuild(_ context.Context, id int64) ([]byte, error) {
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

func (db *DB) GetGuildCount(_ context.Context) (int, error) {
	rr, _ := fdb.PrefixRange(db.fmtGuildPrefix())
	return db.keyCountForPrefix(rr)
}

func (db *DB) GetGuildIDsAfter(_ context.Context, after int64, limit int) ([]int64, error) {
	if limit <= 0 {
		return nil, nil
	}

	prefixRange, err := fdb.PrefixRange(db.fmtGuildPrefix())
	if err != nil {
		return nil, err
	}

	begin := fdb.FirstGreaterThan(db.fmtGuildKey(after))
	end := fdb.FirstGreaterOrEqual(prefixRange.End)
	r := fdb.SelectorRange{Begin: begin, End: end}

	out := make([]int64, 0, limit)
	err = db.ReadTransact(func(t fdb.ReadTransaction) error {
		out = out[:0]
		ropt := fdb.RangeOptions{Mode: fdb.StreamingModeWantAll, Limit: limit}
		it := t.Snapshot().GetRange(r, ropt).Iterator()
		for it.Advance() {
			kv, err := it.Get()
			if err != nil {
				return err
			}
			tup, err := db.subs.Guilds.Unpack(kv.Key)
			if err != nil {
				return err
			}
			if len(tup) == 0 {
				continue
			}
			id, ok := tup[0].(int64)
			if !ok {
				// Non-guild-row key (e.g. nested ban entries with extra
				// tuple components); skip but don't fail.
				continue
			}
			out = append(out, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (db *DB) DeleteGuild(_ context.Context, id int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildKey(id))
		return nil
	})
}

func (db *DB) SetGuildBan(_ context.Context, guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildBanKey(guild, user), raw)
		return nil
	})
}

func (db *DB) GetGuildBan(_ context.Context, guild, user int64) ([]byte, error) {
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

func (db *DB) DeleteGuildBan(_ context.Context, guild, user int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildBanKey(guild, user))
		return nil
	})
}
