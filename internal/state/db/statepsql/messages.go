package statepsql

import "context"

func (db *db) SetChannelMessage(ctx context.Context, channel, id int64, raw []byte) error {
	return nil
}

func (db *db) GetChannelMessage(ctx context.Context, channel, id int64) ([]byte, error) {
	return nil, nil
}

func (db *db) DeleteChannelMessage(ctx context.Context, channel, id int64) error {
	return nil
}

func (db *db) SetChannelMessageReaction(ctx context.Context, channel, id, user int64, name interface{}, raw []byte) error {
	return nil
}

func (db *db) DeleteChannelMessageReaction(ctx context.Context, channel, id, user int64, name interface{}) error {
	return nil
}

func (db *db) DeleteChannelMessageReactions(ctx context.Context, channel, id, user int64) error {
	return nil
}
