package statepsql

import (
	"context"
	"unsafe"

	"golang.org/x/xerrors"
)

func (db *db) SetGuildEmojis(ctx context.Context, guild int64, emojis map[int64][]byte) error {
	const q = `
INSERT INTO
	emojis (id, guild_id, data)
VALUES
	($1, $2, $3)
ON CONFLICT
	(id, guild_id)
DO UPDATE SET
	data = $3
`

	st, err := db.sql.PrepareContext(ctx, q)
	if err != nil {
		return xerrors.Errorf("prepare copy: %w", err)
	}

	for i, e := range emojis {
		_, err := st.ExecContext(ctx, i, guild, e)
		if err != nil {
			return xerrors.Errorf("copy: %w", err)
		}
	}

	err = st.Close()
	if err != nil {
		return xerrors.Errorf("close stmt: %w", err)
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
