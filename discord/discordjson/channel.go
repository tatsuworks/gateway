package discordjson

import (
	"github.com/tatsuworks/gateway/discord"
	"golang.org/x/xerrors"
)

func (_ decoder) DecodeChannel(buf []byte) (*discord.Channel, error) {
	id, err := snowflakeFromObject(buf, "id")
	if err != nil {
		return nil, xerrors.Errorf("extract id from channel: %w", err)
	}

	guildID, err := snowflakeFromObjectOptional(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild_id from channel: %w", err)
	}

	return &discord.Channel{
		ID:      id,
		GuildID: guildID,
		Raw:     buf,
	}, nil
}

func (_ decoder) DecodeVoiceState(buf []byte) (*discord.VoiceState, error) {
	userID, err := snowflakeFromObject(buf, "user_id")
	if err != nil {
		return nil, xerrors.Errorf("extract user_id from voice state: %w", err)
	}

	guildID, err := snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild_id from voice state: %w", err)
	}

	return &discord.VoiceState{
		UserID:  userID,
		GuildID: guildID,
		Raw:     buf,
	}, nil
}
