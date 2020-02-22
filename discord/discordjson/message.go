package discordjson

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeMessage(buf []byte) (*discord.Message, error) {
	var (
		m   discord.Message
		err error
	)

	m.ID, err = snowflakeFromObject(buf, "id")
	if err != nil {
		return nil, xerrors.Errorf("extract message id: %w", err)
	}

	m.ChannelID, err = snowflakeFromObject(buf, "channel_id")
	if err != nil {
		return nil, xerrors.Errorf("extract channel id: %w", err)
	}

	return &m, nil
}
func (_ decoder) DecodeMessageReaction(buf []byte) (*discord.MessageReaction, error) {
	panic("unimplemented")
}
func (_ decoder) DecodeMessageReactionRemoveAll(buf []byte) (*discord.MessageReactionRemoveAll, error) {
	panic("unimplemented")
}
