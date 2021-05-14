package statepsql

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

func (db *db) SetThread(ctx context.Context, guildID, parentID, id int64, raw []byte) error {
	const q = `
			INSERT INTO
				threads (id, parent_id, guild_id, data)
			VALUES
				($1, $2, $3, $4)
			ON CONFLICT (id, parent_id, guild_id)
			DO UPDATE
			SET
				data = $4
			`

	_, err := db.sql.ExecContext(ctx, q, id, guildID, parentID, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetThread(ctx context.Context, id int64) ([]byte, error) {
	const q = `
			SELECT
				data || jsonb_build_object('guild_id', guild_id::text)
			FROM
				threads
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

func (db *db) DeleteThread(ctx context.Context, guild, id int64) error {
	const q = `
			DELETE FROM
				threads
			WHERE
				id = $1
			`

	_, err := db.sql.ExecContext(ctx, q, id)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}

func (db *db) GetThreadsCount(ctx context.Context) (int, error) {
	const q = `
			SELECT
				COUNT(*)
			FROM
				threads
			`

	var c int
	err := db.sql.GetContext(ctx, &c, q)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) SetThreads(ctx context.Context, guildID int64, threads map[int64][]byte) error {
	var q strings.Builder

	q.WriteString(`
			INSERT INTO
				threads (id, parent_id, guild_id, data)
			VALUES 
			`)

	first := true
	for i, e := range threads {
		if !first {
			q.WriteString(", ")
		}
		first = false

		q.WriteString("(" + strconv.FormatInt(i, 10) + ", " + strconv.FormatInt(guildID, 10) + ", " + pq.QuoteLiteral(bytesToString(e)) + "::jsonb)")
	}

	q.WriteString(`
			ON CONFLICT
				(id, parent_id, guild_id)
			DO UPDATE SET
				data = excluded.data
			`)

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

func (db *db) GetThreads(ctx context.Context) ([][]byte, error) {
	const q = `
			SELECT
				data || jsonb_build_object('guild_id', guild_id::text)
			FROM
				threads
			`

	var cs []RawJSON
	err := db.sql.SelectContext(ctx, &cs, q)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[][]byte)(unsafe.Pointer(&cs)), nil
}

func (db *db) GetGuildThreads(ctx context.Context, guild int64) ([][]byte, error) {
	const q = `
			SELECT
				data || jsonb_build_object('guild_id', guild_id::text)
			FROM
				threads
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

func (db *db) DeleteThreads(ctx context.Context, guild int64) error {
	const q = `
			DELETE FROM
				threads
			WHERE
				guild_id = $1
			`
	_, err := db.sql.ExecContext(ctx, q, guild)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}