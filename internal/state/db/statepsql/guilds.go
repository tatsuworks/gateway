package statepsql

import (
	"context"

	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

func (db *db) SetGuild(ctx context.Context, id int64, raw []byte) error {
	return db.guildBatcher.Send(ctx, GuildEvent{GuildID: id, Raw: raw})
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
	count(*)
FROM
	guilds
`

	var c int
	err := db.sql.GetContext(ctx, &c, q)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w", err)
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
		return xerrors.Errorf("exec delete: %w", err)
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

func (db *db) processGuildBatch(ctx context.Context, events []GuildEvent) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const insertQ = `
INSERT INTO guilds (id, data)
SELECT * FROM UNNEST($1::bigint[], $2::jsonb[])
ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data
`

	ids := make([]int64, len(events))
	datas := make([]string, len(events))

	for i, ev := range events {
		ids[i] = ev.GuildID
		datas[i] = string(ev.Raw)
	}
	if _, err := tx.ExecContext(ctx, insertQ, pq.Array(ids), pq.Array(datas)); err != nil {
		return err
	}
	return tx.Commit()
}
