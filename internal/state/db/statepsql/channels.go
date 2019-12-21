package statepsql

import (
	"context"

	"golang.org/x/xerrors"
)

func (db *db) SetChannel(ctx context.Context, _, id int64, raw []byte) error {
	const q = `
INSERT INTO
	channels (id, data)
VALUES
	($1, $2)
`

	_, err := db.sql.ExecContext(ctx, q, id, raw)
	if err != nil {
		return xerrors.Errorf("exec insert: %w", err)
	}

	return nil
}

func (db *db) GetChannel(ctx context.Context, id int64) ([]byte, error) {
	const q = `
SELECT
	data
FROM
	channels
WHERE
	id = $1
`

	c := RawJSON{}
	err := db.sql.SelectContext(ctx, &c, q)
	if err != nil {
		return nil, xerrors.Errorf("exec select: %w")
	}

	return c, nil
}

func (db *db) GetChannelCount(ctx context.Context) (int, error) {
	const q = `
SELECT
	COUNT(*)
FROM
	channels
`

	var c int
	err := db.sql.SelectContext(ctx, &c, q)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w")
	}

	return c, nil
}

func (db *db) GetChannels(ctx context.Context) ([][]byte, error) {
	const q = `
SELECT
	COUNT(*)
FROM
	channels
`

	var c int
	err := db.sql.SelectContext(ctx, &c, q)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w")
	}

	return c, nil
}

func (db *db) GetGuildChannels(ctx context.Context, guild int64) ([]map[int64][]byte, error) {
	return nil, nil
}

func (db *db) DeleteChannel(ctx context.Context, guild, id int64, raw []byte) error {
	return nil
}

func (db *db) SetChannels(ctx context.Context, guild int64, channels map[int64][]byte) error {
	return nil
}

func (db *db) DeleteChannels(ctx context.Context, guild int64) error {
	return nil
}

func (db *db) SetVoiceState(ctx context.Context, guild, user int64, raw []byte) error {
	return nil
}
