package statepsql

import "context"

func (db *db) SetGuildMembers(ctx context.Context, guild int64, raws map[int64][]byte) error {
	return nil
}

func (db *db) DeleteGuildMembers(ctx context.Context, guild int64) error {
	return nil
}

func (db *db) SetGuildMember(ctx context.Context, guild, user int64, raw []byte) error {
	return nil
}

func (db *db) GetGuildMember(ctx context.Context, guild, user int64) ([]byte, error) {
	return nil, nil
}

func (db *db) GetGuildMembers(ctx context.Context, guild int64) ([]map[int64][]byte, error) {
	return nil, nil
}

func (db *db) DeleteGuildMember(ctx context.Context, guild, user int64) error {
	return nil
}
