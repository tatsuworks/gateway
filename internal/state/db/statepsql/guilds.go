package statepsql

import "context"

func (db *db) SetGuild(ctx context.Context, id int64, raw []byte) error {
	return nil
}

func (db *db) GetGuild(ctx context.Context, id int64) ([]byte, error) {
	return nil, nil
}

func (db *db) GetGuildCount(ctx context.Context) (int, error) {
	return 0, nil
}

func (db *db) DeleteGuild(ctx context.Context, id int64) error {
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
