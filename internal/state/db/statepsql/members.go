package statepsql

import (
	"context"
	"fmt"
	"unsafe"

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
	members
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
	const q = `
INSERT INTO
	members (user_id, guild_id, data)
VALUES
	($1, $2, $3)
ON CONFLICT
	(user_id, guild_id)
DO UPDATE SET
	data = $3
`

	st, err := db.sql.PrepareContext(ctx, q)
	if err != nil {
		return xerrors.Errorf("prepare insert: %w", err)
	}

	for i, e := range members {
		if guildID == 390426490103136256 {
			fmt.Println(i, guildID, string(e))
		}
		_, err := st.ExecContext(ctx, i, guildID, e)
		if err != nil {
			// fmt.Println(e)
			// fmt.Println(xerrors.Errorf("insert member: %w", err).Error())
			return xerrors.Errorf("insert member: %w", err)
		}
	}

	err = st.Close()
	if err != nil {
		return xerrors.Errorf("close statement: %w", err)
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
	guild_id = $1
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

func (db *db) GetUser(ctx context.Context, userID int64) ([]byte, error) {
	const q = `
SELECT
	data->'user'
FROM
	members
WHERE
	user_id = $1
LIMIT 1
`

	var usr RawJSON
	err := db.sql.SelectContext(ctx, &usr, q, userID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[]byte)(unsafe.Pointer(&usr)), nil
}
