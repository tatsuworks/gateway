package statepsql

import "context"

func (db *db) SetGuildRole(ctx context.Context, guild, role int64, raw []byte) error {
	return nil
}

func (db *db) GetGuildRole(ctx context.Context, guild, role int64) ([]byte, error) {
	return nil, nil
}

func (db *db) SetGuildRoles(ctx context.Context, guild int64, roles map[int64][]byte) error {
	return nil
}

func (db *db) GetGuildRoles(ctx context.Context, guild int64) ([][]byte, error) {
	return nil, nil
}

func (db *db) DeleteGuildRoles(ctx context.Context, guild int64) error {
	return nil
}

func (db *db) DeleteGuildRole(ctx context.Context, guild, role int64) error {
	return nil
}
