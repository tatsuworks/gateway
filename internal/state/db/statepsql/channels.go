package statepsql

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

func (db *db) SetChannel(ctx context.Context, guildID, id int64, raw []byte) error {
	const q = `
INSERT INTO
	channels (id, guild_id, data)
VALUES
	($1, $2, $3)
ON CONFLICT (id, guild_id)
DO UPDATE
SET
	data = $3
`

	_, err := db.sql.ExecContext(ctx, q, id, guildID, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetChannel(ctx context.Context, id int64) ([]byte, error) {
	const q = `
SELECT
	data || jsonb_build_object('guild_id', guild_id::text)
FROM
	channels
WHERE
	id = $1
`

	c := RawJSON{}
	err := db.sql.GetContext(ctx, &c, q, id)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) DeleteChannel(ctx context.Context, guild, id int64) error {
	const q = `
DELETE FROM
	channels
WHERE
	id = $1
`

	_, err := db.sql.ExecContext(ctx, q, id)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}

func (db *db) GetChannelCount(ctx context.Context) (int, error) {
	const q = `
SELECT
	COUNT(*)
FROM
	channels
`

	var c int
	err := db.sql.GetContext(ctx, &c, q)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) SetChannels(ctx context.Context, guildID int64, channels map[int64][]byte) error {
	var q strings.Builder

	q.WriteString(`
INSERT INTO
	channels (id, guild_id, data)
VALUES 
`)

	first := true
	for i, e := range channels {
		if !first {
			q.WriteString(", ")
		}
		first = false

		q.WriteString("(" + strconv.FormatInt(i, 10) + ", " + strconv.FormatInt(guildID, 10) + ", " + pq.QuoteLiteral(bytesToString(e)) + "::jsonb)")
	}

	q.WriteString(`
ON CONFLICT
	(id, guild_id)
DO UPDATE SET
	data = excluded.data
`)

	fmt.Println("q.String()", q.String())
	_, err := db.sql.ExecContext(ctx, q.String())
	if err != nil {
		return xerrors.Errorf("copy: %w", err)
	}

	return nil
}

func bytesToString(b []byte) string {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := reflect.StringHeader{Data: bh.Data, Len: bh.Len}
	str := *(*string)(unsafe.Pointer(&sh))
	return strings.ToValidUTF8(str, "")
}

func (db *db) GetChannels(ctx context.Context) ([][]byte, error) {
	const q = `
SELECT
	data || jsonb_build_object('guild_id', guild_id::text)
FROM
	channels
`

	var cs []RawJSON
	err := db.sql.SelectContext(ctx, &cs, q)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[][]byte)(unsafe.Pointer(&cs)), nil
}

func (db *db) GetGuildChannels(ctx context.Context, guild int64) ([][]byte, error) {
	const q = `
SELECT
	data || jsonb_build_object('guild_id', guild_id::text)
FROM
	channels
WHERE
	guild_id = $1
`

	var cs []RawJSON
	err := db.sql.SelectContext(ctx, &cs, q, guild)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[][]byte)(unsafe.Pointer(&cs)), nil
}

func (db *db) DeleteChannels(ctx context.Context, guild int64) error {
	const q = `
DELETE FROM
	channels
WHERE
	guild_id = $1
`
	_, err := db.sql.ExecContext(ctx, q, guild)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}

func (db *db) SetVoiceState(ctx context.Context, guild, user int64, raw []byte) error {
	return nil
}
