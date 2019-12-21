package statepsql

import (
	"context"

	"golang.org/x/xerrors"
)

func (db *db) SetGuild(ctx context.Context, id int64, raw []byte) error {
	const q = `
INSERT INTO
	guilds (id, data)
VALUES
	($1, $2)
ON CONFLICT (id)
DO UPDATE
SET
	data = $2
`

	_, err := db.sql.ExecContext(ctx, q, id, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetGuild(ctx context.Context, id int64) ([]byte, error) {
	const q = `
SELECT
	data
FROM
	guilds
WHERE
	id = $1
`

	g := RawJSON{}
	err := db.sql.GetContext(ctx, &g, q, id)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return g, nil
}

func (db *db) GetGuildCount(ctx context.Context) (int, error) {
	const q = `
SELECT
	COUNT(*)
FROM
	guilds
`

	var c int
	err := db.sql.GetContext(ctx, &c, q)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w")
	}

	return c, nil
}

func (db *db) DeleteGuild(ctx context.Context, id int64) error {
	const q = `
DELETE FROM
	guilds
WHERE
	id = $1
`

	_, err := db.sql.ExecContext(ctx, q, id)
	if err != nil {
		return xerrors.Errorf("exec delete: %w")
	}

	return nil
}

func (db *db) SetGuildBan(ctx context.Context, guild, user int64, raw []byte) error {
	return nil
}

func (db *db) GetGuildBan(ctx context.Context, guild, user int64) ([]byte, error) {
	return nil, nil
}

func (db *db) DeleteGuildBan(ctx context.Context, guild, user int64) error {
	return nil
}
