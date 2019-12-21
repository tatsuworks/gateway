package statefdb

import (
	"context"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"golang.org/x/xerrors"
)

func (db *DB) SetChannel(ctx context.Context, guild, id int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtChannelKey(id), raw)
		t.Set(db.fmtGuildChannelKey(guild, id), raw)
		return nil
	})
}

func (db *DB) GetChannel(ctx context.Context, id int64) ([]byte, error) {
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

func (db *DB) GetChannelCount(ctx context.Context) (int, error) {
	rr, _ := fdb.PrefixRange(db.fmtChannelPrefix())
	return db.keyCountForPrefix(rr)
}

func (db *DB) keyCountForPrefix(r fdb.Range) (int, error) {
	var count int

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		count = 0

		ropt := fdb.RangeOptions{Mode: fdb.StreamingModeSerial}
		i := t.Snapshot().GetRange(r, ropt).Iterator()

		for i.Advance() {
			count++
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (db *DB) GetChannels(ctx context.Context) ([][]byte, error) {
	var (
		raws   []fdb.KeyValue
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtChannelPrefix())
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to read channels: %w", err)
	}

	out = make([][]byte, len(raws))
	for i, e := range raws {
		out[i] = e.Value
	}

	return out, err
}

func (db *DB) GetGuildChannels(ctx context.Context, guild int64) ([][]byte, error) {
	var (
		raws   []fdb.KeyValue
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtGuildChannelPrefix(guild))
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to read channels: %w", err)
	}

	out = make([][]byte, len(raws))
	for i, e := range raws {
		out[i] = e.Value
	}

	return out, err
}

func (db *DB) DeleteChannel(ctx context.Context, guild, id int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtChannelKey(id))
		t.Clear(db.fmtGuildChannelKey(guild, id))
		return nil
	})
}

func (db *DB) SetChannels(ctx context.Context, guild int64, channels map[int64][]byte) error {
	return db.setETFs(channels, db.fmtChannelKey)
}

// this will leak channels in the main pool.
// TODO: fix
func (db *DB) DeleteChannels(ctx context.Context, guild int64) error {
	gRange, _ := fdb.PrefixRange(db.fmtGuildChannelPrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(gRange)
		return nil
	})
}

func (db *DB) SetVoiceState(ctx context.Context, guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Set(db.fmtGuildVoiceStateKey(guild, user), raw)
		return nil
	})
}
