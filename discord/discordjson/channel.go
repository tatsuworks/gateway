package discordjson

import (
	jsoniter "github.com/json-iterator/go"
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

	channelID, err := snowflakeFromObjectOptional(buf, "channel_id")
	if err != nil {
		return nil, xerrors.Errorf("extract channel_id from voice state: %w", err)
	}

	return &discord.VoiceState{
		UserID:    userID,
		GuildID:   guildID,
		ChannelID: channelID,
		IsBot:     VoiceStateIsBot(buf),
		Raw:       buf,
	}, nil
}

// VoiceStateIsBot classifies the account behind a voice state from its member
// object. Discord omits user.bot for humans, so a present user object without
// it is a human; a missing user object is unknown, never assumed either way.
func VoiceStateIsBot(raw []byte) *bool {
	return UserIsBot(jsoniter.Get(raw, "member"))
}

// MemberIsBot classifies a guild member object the same way.
func MemberIsBot(raw []byte) *bool {
	return UserIsBot(jsoniter.Get(raw))
}

// UserIsBot reads user.bot from an object that nests a user.
func UserIsBot(member jsoniter.Any) *bool {
	user := member.Get("user")
	if user.ValueType() != jsoniter.ObjectValue {
		return nil
	}
	isBot := user.Get("bot").ValueType() == jsoniter.BoolValue && user.Get("bot").ToBool()
	return &isBot
}

// ChannelIDOf reads a voice state's channel_id; 0 means not in a channel.
func ChannelIDOf(raw []byte) (int64, error) {
	return snowflakeFromObjectOptional(raw, "channel_id")
}
