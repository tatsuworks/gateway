package statepsql

import (
	"context"
	"unsafe"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

func (db *db) SetGuildMember(ctx context.Context, guildID, userID int64, raw []byte) error {
	const q = `
INSERT INTO
	members (user_id, guild_id, data)
VALUES
	($1, $2, $3)
ON CONFLICT (user_id, guild_id)
DO UPDATE
SET
	data = $3
`

	_, err := db.sql.ExecContext(ctx, q, userID, guildID, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetGuildMember(ctx context.Context, guildID, userID int64) ([]byte, error) {
	const q = `
SELECT
	data
FROM
	members
WHERE
	guild_id = $1 AND
	user_id = $2
`

	c := RawJSON{}
	err := db.sql.GetContext(ctx, &c, q, guildID, userID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) DeleteGuildMember(ctx context.Context, guildID, userID int64) error {
	const q = `
DELETE FROM
	guild_members
WHERE
	guild_id = $1 AND
	user_id = $2
`

	_, err := db.sql.ExecContext(ctx, q, guildID, userID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w")
	}

	return nil
}

func (db *db) SetGuildMembers(ctx context.Context, guildID int64, members map[int64][]byte) error {
	txn, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return xerrors.Errorf("start txn: %w", err)
	}
	defer txn.Rollback()

	const q = `
DELETE FROM
	members
WHERE
	guild_id = $1
`
	_, err = txn.ExecContext(ctx, q, guildID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	st, err := txn.PrepareContext(ctx, pq.CopyIn("channels", "guild_id", "user_id", "data"))
	if err != nil {
		return xerrors.Errorf("prepare copy: %w", err)
	}

	for i, e := range members {
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

func (db *db) GetGuildMembers(ctx context.Context, guildID int64) ([][]byte, error) {
	const q = `
SELECT
	data
FROM
	members
WHERE
	guild_id $1
`

	var ms []RawJSON
	err := db.sql.SelectContext(ctx, &ms, q, guildID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w")
	}

	return *(*[][]byte)(unsafe.Pointer(&ms)), nil
}

func (db *db) DeleteGuildMembers(ctx context.Context, guildID int64) error {
	const q = `
DELETE FROM
	members
WHERE
	guild_id = $1
`
	_, err := db.sql.ExecContext(ctx, q, guildID)
	if err != nil {
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}
