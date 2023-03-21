package statefdb

import "context"

func (db *DB) GetShardInfo(ctx context.Context, shard int, name string) (sess string, seq int64, err error) {
	panic("unimplemented")
}
func (db *DB) SetSequence(ctx context.Context, shard int, name string, seq int64) error {
	panic("unimplemented")
}
func (db *DB) GetSequence(ctx context.Context, shard int, name string) (int64, error) {
	panic("unimplemented")
}
func (db *DB) SetSessionID(ctx context.Context, shard int, name, sess string) error {
	panic("unimplemented")
}
func (db *DB) GetSessionID(ctx context.Context, shard int, name string) (string, error) {
	panic("unimplemented")
}
func (db *DB) SetStatus(ctx context.Context, shard int, name, sess string) error {
	panic("unimplemented")
}
func (db *DB) SetResumeGatewayURL(ctx context.Context, shard int, name string, resumeURL string) error {
	panic("unimplemented")
}
func (db *DB) GetResumeGatewayURL(ctx context.Context, shard int, name string) (string, error) {
	panic("unimplemented")
}
