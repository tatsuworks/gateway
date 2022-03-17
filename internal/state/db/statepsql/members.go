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
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return c, nil
}

func (db *db) GetGuildMemberCount(ctx context.Context, guildID int64) (int, error) {
	const q = `
SELECT
	count(*)
FROM
	members
WHERE
	guild_id = $1
`

	var mc int
	err := db.sql.GetContext(ctx, &mc, q, guildID)
	if err != nil {
		return 0, xerrors.Errorf("exec get: %w", err)
	}

	return mc, nil
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
		return xerrors.Errorf("exec delete: %w", err)
	}

	return nil
}

func (db *db) SetGuildMembers(ctx context.Context, guildID int64, members map[int64][]byte) error {
	var q strings.Builder

	q.WriteString(`
INSERT INTO
	members (user_id, guild_id, data)
VALUES
`)

	first := true
	for i, e := range members {
		if !first {
			q.WriteString(", ")
		}
		first = false

		q.WriteString("(" + strconv.FormatInt(i, 10) + ", " + strconv.FormatInt(guildID, 10) + ", " + pq.QuoteLiteral(bytesToString(e)) + "::jsonb)")
	}

	q.WriteString(`
ON CONFLICT
	(user_id, guild_id)
DO UPDATE SET
	data = excluded.data
`)

	_, err := db.sql.ExecContext(ctx, q.String())
	if err != nil {
		return xerrors.Errorf("copy: %w", err)
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
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[][]byte)(unsafe.Pointer(&ms)), nil
}

func (db *db) GetGuildMembersWithRole(ctx context.Context, guildID, roleID int64) ([][]byte, error) {
	const q = `select count(*) from members where guild_id = $1 and  data->'roles' ? $2`

	var ms []RawJSON
	err := db.sql.SelectContext(ctx, &ms, q, guildID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
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
	q := `
SELECT
	data->'user'
FROM
	members
WHERE
	user_id = $1
ORDER BY last_updated desc nulls last limit 1
`

	var usr RawJSON
	err := db.sql.GetContext(ctx, &usr, q, userID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}
	return *(*[]byte)(unsafe.Pointer(&usr)), nil
}

func (db *db) SearchGuildMembers(ctx context.Context, guildID int64, query string) ([][]byte, error) {
	const q = `
SELECT
	data
FROM
	members
WHERE
	guild_id = $1 AND (
		data->'user'->>'username' ilike $2 OR
		data->>'nick' ilike $2
	)
`

	var ms []RawJSON
	err := db.sql.SelectContext(ctx, &ms, q, guildID, "%"+query+"%")
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}

	return *(*[][]byte)(unsafe.Pointer(&ms)), nil
}

func (db *db) SetPresence(ctx context.Context, guildID, userID int64, raw []byte) error {
	const q = `
INSERT INTO
	presence (user_id, guild_id, data)
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

func (db *db) GetUserPresence(ctx context.Context, guildID, userID int64) ([]byte, error) {
	q := `
SELECT
	data
FROM
	presence
WHERE
	user_id = $1 AND guild_id = $2
`

	var presence RawJSON
	err := db.sql.GetContext(ctx, &presence, q, userID, guildID)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w", err)
	}
	return *(*[]byte)(unsafe.Pointer(&presence)), nil
}

func (db *db) SetPresences(ctx context.Context, guildID int64, presences map[int64][]byte) error {
	var q strings.Builder

	q.WriteString(`
			INSERT INTO
				presence (user_id, guild_id, data)
			VALUES 
			`)

	first := true
	for i, e := range presences {
		if !first {
			q.WriteString(", ")
		}
		first = false

		q.WriteString("(" + strconv.FormatInt(i, 10) + ", " + strconv.FormatInt(guildID, 10) + ", " + pq.QuoteLiteral(bytesToString(e)) + "::jsonb)")
	}

	q.WriteString(`
			ON CONFLICT
				(user_id, guild_id)
			DO UPDATE SET
				data = excluded.data
			`)

	_, err := db.sql.ExecContext(ctx, q.String())
	if err != nil {
		return xerrors.Errorf("copy: %w", err)
	}

	return nil
}
