package statepsql

import (
	"context"

	"golang.org/x/xerrors"
)

func (db *db) GetShardInfo(ctx context.Context, shard int, name string) (sess string, seq int64, err error) {
	sess, err = db.GetSessionID(ctx, shard, name)
	if err != nil {
		return "", 0, xerrors.Errorf("get session id: %w", err)
	}

	seq, err = db.GetSequence(ctx, shard, name)
	if err != nil {
		return "", 0, xerrors.Errorf("get sequence: %w", err)
	}

	return
}

func (db *db) SetSequence(ctx context.Context, shard int, name string, seq int64) error {
	const q = `
INSERT INTO
	shards (id, name, seq, sess)
VALUES
	($1, $2, $3, '')
ON CONFLICT
	(id, name)
DO UPDATE
SET
	seq = $3
`

	_, err := db.sql.ExecContext(ctx, q, shard, name, seq)
	if err != nil {
		return xerrors.Errorf("exec update: %w", err)
	}

	return nil
}

func (db *db) GetSequence(ctx context.Context, shard int, name string) (int64, error) {
	const q = `
SELECT
	seq
FROM
	shards
WHERE
	id = $1 AND
	name = $2
`

	var seq int64
	err := db.sql.GetContext(ctx, &seq, q, shard, name)
	if err != nil {
		return 0, xerrors.Errorf("exec select: %w", err)
	}

	return seq, nil
}

func (db *db) SetSessionID(ctx context.Context, shard int, name, sess string) error {
	const q = `
INSERT INTO
	shards (id, name, seq, sess)
VALUES
	($1, $2, 0, $3)
ON CONFLICT
	(id, name)
DO UPDATE
SET
	sess = $3
`

	_, err := db.sql.ExecContext(ctx, q, shard, name, sess)
	if err != nil {
		return xerrors.Errorf("exec update: %w", err)
	}

	return nil
}

func (db *db) GetSessionID(ctx context.Context, shard int, name string) (string, error) {
	const q = `
SELECT
	sess
FROM
	shards
WHERE
	id = $1 AND
	name = $2
`

	var sess string
	err := db.sql.GetContext(ctx, &sess, q, shard, name)
	if err != nil {
		return "", xerrors.Errorf("exec select: %w", err)
	}

	return sess, nil
}

func (db *db) SetStatus(ctx context.Context, shard int, name, status string) error {
	const q = `
INSERT INTO
	shards (id, name, status)
VALUES
	($1, $2)
ON CONFLICT
	(id, name)
DO UPDATE
SET
status = $3
`

	_, err := db.sql.ExecContext(ctx, q, shard, name, status)
	if err != nil {
		return xerrors.Errorf("exec update: %w", err)
	}

	return nil
}
