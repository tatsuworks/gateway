package discordjson

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/tatsuworks/gateway/discord"
	"golang.org/x/xerrors"
)

func (_ decoder) DecodeGuildEmojisUpdate(buf []byte) (*discord.GuildEmojisUpdate, error) {
	var (
		eu  discord.GuildEmojisUpdate
		err error
	)

	var emojis []jsoniter.RawMessage
	jsoniter.Get(buf, "emojis").ToVal(&emojis)
	eu.Emojis, err = rawsToMapBySnowflake(emojis, "id")
	if err != nil {
		return nil, xerrors.Errorf("map emojis by id: %w", err)
	}

	eu.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	return &eu, nil
}
