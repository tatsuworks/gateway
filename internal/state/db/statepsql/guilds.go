package statepsql

func (db *db) SetGuild(id int64, raw []byte) error {
	return nil
}

func (db *db) GetGuild(id int64) ([]byte, error) {
	return nil, nil
}

func (db *db) GetGuildCount() (int, error) {
	return 0, nil
}

func (db *db) DeleteGuild(id int64) error {
	return nil
}

func (db *db) SetGuildBan(guild, user int64, raw []byte) error {
	return nil
}

func (db *db) GetGuildBan(guild, user int64) ([]byte, error) {
	return nil, nil
}

func (db *db) DeleteGuildBan(guild, user int64) error {
	return nil
}
