package statepsql

func (db *db) SetChannel(guild, id int64, raw []byte) error {
	return nil
}

func (db *db) GetChannel(id int64) ([]byte, error) {
	return nil, nil
}

func (db *db) GetChannelCount() (int, error) {
	return 0, nil
}

func (db *db) GetChannels() ([]map[int64][]byte, error) {
	return nil, nil
}

func (db *db) GetGuildChannels(guild int64) ([]map[int64][]byte, error) {
	return nil, nil
}

func (db *db) DeleteChannel(guild, id int64, raw []byte) error {
	return nil
}

func (db *db) SetChannels(guild int64, channels map[int64][]byte) error {
	return nil
}

func (db *db) DeleteChannels(guild int64) error {
	return nil
}

func (db *db) SetVoiceState(guild, user int64, raw []byte) error {
	return nil
}
