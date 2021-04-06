package statefdb

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
)

func (db *DB) SetGuildMembers(_ context.Context, guild int64, raws map[int64][]byte) error {
	err := db.Transact(func(t fdb.Transaction) error {
		for k, v := range raws {
			err := db.SetGuildMemberInTxn(t, guild, k, v)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func (db *DB) DeleteGuildMembers(_ context.Context, guild int64) error {
	pre, _ := fdb.PrefixRange(db.fmtGuildMemberPrefix(guild))

	return db.Transact(func(t fdb.Transaction) error {
		t.ClearRange(pre)
		return nil
	})
}

func (db *DB) SetGuildMemberInTxn(t fdb.Transaction, guild, user int64, raw []byte) error {
	t.Set(db.fmtGuildMemberKey(guild, user), raw)
	t.Set(db.fmtMemberGuildKey(guild, user), int64ToBytes(time.Now().Unix()))

	return nil
}

func (db *DB) SetGuildMember(_ context.Context, guild, user int64, raw []byte) error {
	return db.Transact(func(t fdb.Transaction) error {
		err := db.SetGuildMemberInTxn(t, guild, user, raw)
		if err != nil {
			return err
		}
		return nil
	})
}

func (db *DB) GetGuildMember(_ context.Context, guild, user int64) ([]byte, error) {
	var m []byte

	err := db.Transact(func(t fdb.Transaction) error {
		m = t.Get(db.fmtGuildMemberKey(guild, user)).MustGet()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (db *DB) GetGuildMembers(_ context.Context, guild int64) ([][]byte, error) {
	var (
		raws   []fdb.KeyValue
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtGuildMemberPrefix(guild))
	)

	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return nil, err
	}

	out = make([][]byte, len(raws))
	for i, e := range raws {
		out[i] = e.Value
	}

	return out, err
}

func (db *DB) DeleteGuildMember(_ context.Context, guild, user int64) error {
	return db.Transact(func(t fdb.Transaction) error {
		t.Clear(db.fmtGuildMemberKey(guild, user))
		t.Clear(db.fmtMemberGuildKey(guild, user))
		return nil
	})
}
func (db *DB) GetUser(ctx context.Context, userID int64) ([]byte, error) {
	var (
		m []byte
	)
	rr, err := fdb.PrefixRange(db.fmtMemberGuildPrefix(userID))
	if err != nil {
		return nil, err
	}
	err = db.Transact(func(t fdb.Transaction) error {
		raws := t.Snapshot().GetRange(rr, FDBRangeWantAll).GetSliceOrPanic()

		latestTime := int64(0)
		var keyToUse fdb.Key
		for _, raw := range raws {
			unixTime := bytesToInt64(raw.Value)
			if unixTime > latestTime {
				keyToUse = raw.Key
				latestTime = unixTime
			}
		}
		if keyToUse == nil {
			return nil
		}
		guild, err := db.guildFromMembersIndexKey(keyToUse)
		if err != nil {
			return err
		}
		m = t.Snapshot().Get(db.fmtGuildMemberKey(guild, userID)).MustGet()
		return nil
	})

	if err != nil {
		return nil, err
	}

	return m, nil
}

type MemberUserData struct {
	Username string `json:"username"`
}
type MemberData struct {
	Nick string         `json:"nick"`
	User MemberUserData `json:"user"`
}

func (db *DB) SearchGuildMembers(ctx context.Context, guildID int64, query string) ([][]byte, error) {
	var (
		out    [][]byte
		pre, _ = fdb.PrefixRange(db.fmtGuildMemberPrefix(guildID))
	)
	err := db.ReadTransact(func(t fdb.ReadTransaction) error {
		ropt := fdb.RangeOptions{Mode: fdb.StreamingModeSerial}
		i := t.Snapshot().GetRange(pre, ropt).Iterator()
		q := strings.ToLower(query)
		for i.Advance() {
			raw := i.MustGet()
			var d MemberData
			err := json.Unmarshal(raw.Value, &d)

			if err != nil {
				return err
			}
			if strings.Contains(strings.ToLower(d.Nick), q) || strings.Contains(strings.ToLower(d.User.Username), q) {
				out = append(out, raw.Value)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return out, err
}

func (db *DB) GetGuildMemberCount(ctx context.Context, guildID int64) (int, error) {
	rr, _ := fdb.PrefixRange(db.fmtGuildMemberPrefix(guildID))
	return db.keyCountForPrefix(rr)

}
