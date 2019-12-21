package statepsql

import (
	"context"
	"unsafe"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

func (db *db) SetGuildRole(ctx context.Context, guildID, roleID int64, raw []byte) error {
	const q = `
INSERT INTO
	role (guild_id, role_id data)
VALUES
	($1, $2, $3)
ON CONFLICT (guild_id, role_id)
DO UPDATE
SET
	data = $3
`

	_, err := db.sql.ExecContext(ctx, q, guildID, roleID, raw)
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
	guild_id = $1 AND
	role_id = $2
`

	c := RawJSON{}
	err := db.sql.GetContext(ctx, &c, q, guildID, roleID)
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
	guild_id = $1 AND
	role_id = $2
`

	_, err := db.sql.ExecContext(ctx, q, guildID, roleID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w")
	}

	return nil
}

func (db *db) SetGuildRoles(ctx context.Context, guildID int64, roles map[int64][]byte) error {
	txn, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("start txn: %w", err)
	}
	defer txn.Rollback()

	const q = `
DELETE FROM
	roles
WHERE
	guild_id = $1
`
	_, err = txn.ExecContext(ctx, q, guildID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	st, err := txn.PrepareContext(ctx, pq.CopyIn("channels", "guild_id", "role_id", "data"))
	if err != nil {
		return xerrors.Errorf("prepare copy: %w", err)
	}

	for i, e := range roles {
		_, err := st.ExecContext(ctx, guildID, i, e)
		if err != nil {
			return xerrors.Errorf("copy: %w", err)
		}
	}

	err = st.Close()
	if err != nil {
		return xerrors.Errorf("close prepare: %w", err)
	}

	err = txn.Commit()
	if err != nil {
		return xerrors.Errorf("commit: %w", err)
	}

	return nil
}

func (db *db) GetGuildRoles(ctx context.Context, guildID int64) ([][]byte, error) {
	const q = `
SELECT
	data
FROM
	members
WHERE
	guild_id $1
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
