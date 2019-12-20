package statepsql

func (db *db) SetGuildRole(guild, role int64, raw []byte) error {
	return nil
}

func (db *db) GetGuildRole(guild, role int64) ([]byte, error) {
	return nil, nil
}

func (db *db) SetGuildRoles(guild int64, roles map[int64][]byte) error {
	return nil
}

func (db *db) GetGuildRoles(guild int64) ([]map[int64][]byte, error) {
	return nil, nil
}

func (db *db) DeleteGuildRoles(guild int64) error {
	return nil
}

func (db *db) DeleteGuildRole(guild, role int64) error {
	return nil
}
