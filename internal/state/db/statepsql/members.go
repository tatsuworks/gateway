package statepsql

func (db *db) SetGuildMembers(guild int64, raws map[int64][]byte) error {
	return nil
}

func (db *db) DeleteGuildMembers(guild int64) error {
	return nil
}

func (db *db) SetGuildMember(guild, user int64, raw []byte) error {
	return nil
}

func (db *db) GetGuildMember(guild, user int64) ([]byte, error) {
	return nil, nil
}

func (db *db) GetGuildMembers(guild int64) ([]map[int64][]byte, error) {
	return nil, nil
}

func (db *db) DeleteGuildMember(guild, user int64) error {
	return nil
}
