package statepsql

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"unsafe"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

func (db *db) SetGuildEmojis(ctx context.Context, guildID int64, emojis map[int64][]byte) error {
	var q strings.Builder

	q.WriteString(`
INSERT INTO
	emojis (id, guild_id, data)
VALUES
`)

	first := true
	for i, e := range emojis {
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

	_, err := db.sql.ExecContext(ctx, q.String())
	if err != nil {
		return xerrors.Errorf("copy: %w", err)
	}

	return nil
}

func (db *db) SetGuildEmoji(ctx context.Context, guildID, emojiID int64, raw []byte) error {
	const q = `
INSERT INTO
	emojis (id, guild_id, data)
VALUES
	($1, $2, $3)
ON CONFLICT (id, guild_id)
DO UPDATE
SET
	data = $3
`

	_, err := db.sql.ExecContext(ctx, q, emojiID, guildID, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetGuildEmoji(ctx context.Context, guildID, emojiID int64) ([]byte, error) {
	const q = `
SELECT
	data
FROM
	emojis
WHERE
	guild_id = $1 AND
	id = $2
`

	c := RawJSON{}
	err := db.sql.GetContext(ctx, &c, q, guildID, emojiID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) GetGuildEmojis(ctx context.Context, guildID int64) ([][]byte, error) {
	const q = `
SELECT
	data
FROM
	emojis
WHERE
	guild_id = $1
`

	var es []RawJSON
	err := db.sql.SelectContext(ctx, &es, q, guildID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[][]byte)(unsafe.Pointer(&es)), nil
}

func (db *db) DeleteGuildEmoji(ctx context.Context, guildID, emojiID int64) error {
	const q = `
DELETE FROM
	emojis
WHERE
	guild_id = $1 AND
	id = $2
`
	_, err := db.sql.ExecContext(ctx, q, guildID, emojiID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}
