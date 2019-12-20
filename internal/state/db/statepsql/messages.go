package statepsql

func (db *db) SetChannelMessage(channel, id int64, raw []byte) error {
	return nil
}

func (db *db) GetChannelMessage(channel, id int64) ([]byte, error) {
	return nil, nil
}

func (db *db) DeleteChannelMessage(channel, id int64) error {
	return nil
}

func (db *db) SetChannelMessageReaction(channel, id, user int64, name interface{}, raw []byte) error {
	return nil
}

func (db *db) DeleteChannelMessageReaction(channel, id, user int64, name interface{}) error {
	return nil
}

func (db *db) DeleteChannelMessageReactions(channel, id, user int64) error {
	return nil
}
