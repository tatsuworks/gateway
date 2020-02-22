package statepsql

import (
	"context"
	"unsafe"

	"golang.org/x/xerrors"
)

func (db *db) SetGuildRole(ctx context.Context, guildID, roleID int64, raw []byte) error {
	const q = `
INSERT INTO
	roles (id, guild_id, data)
VALUES
	($1, $2, $3)
ON CONFLICT (id, guild_id)
DO UPDATE SET
	data = $3
`

	_, err := db.sql.ExecContext(ctx, q, roleID, guildID, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetGuildRole(ctx context.Context, guildID, roleID int64) ([]byte, error) {
	const q = `
SELECT
	data
FROM
	roles
WHERE
	id = $1 AND
	guild_id = $2
`

	c := RawJSON{}
	err := db.sql.GetContext(ctx, &c, q, roleID, guildID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) DeleteGuildRole(ctx context.Context, guildID, roleID int64) error {
	const q = `
DELETE FROM
	roles
WHERE
	id = $1 AND
	guild_id = $2
`

	_, err := db.sql.ExecContext(ctx, q, roleID, guildID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w")
	}

	return nil
}

func (db *db) SetGuildRoles(ctx context.Context, guildID int64, roles map[int64][]byte) error {
	const q = `
INSERT INTO
	roles (id, guild_id, data)
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

	for i, e := range roles {
		_, err := st.ExecContext(ctx, i, guildID, e)
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

func (db *db) GetGuildRoles(ctx context.Context, guildID int64) ([][]byte, error) {
	const q = `
SELECT
	data
FROM
	roles
WHERE
	guild_id = $1
`

	var rs []RawJSON
	err := db.sql.SelectContext(ctx, &rs, q, guildID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w")
	}

	return *(*[][]byte)(unsafe.Pointer(&rs)), nil
}

func (db *db) DeleteGuildRoles(ctx context.Context, guildID int64) error {
	const q = `
DELETE FROM
	roles
WHERE
	guild_id = $1
`
	_, err := db.sql.ExecContext(ctx, q, guildID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}
