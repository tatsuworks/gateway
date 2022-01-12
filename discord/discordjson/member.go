package discordjson

import (
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeMemberChunk(buf []byte) (*discord.MemberChunk, error) {
	var (
		mc  discord.MemberChunk
		err error
	)

	var members []jsoniter.RawMessage
	jsoniter.Get(buf, "members").ToVal(&members)
	mc.Members, err = nestedRawsToMapBySnowflake(members, "user")
	if err != nil {
		return nil, xerrors.Errorf("map members by id: %w", err)
	}

	mc.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	return &mc, nil
}

func (_ decoder) DecodeMember(buf []byte) (*discord.Member, error) {
	var (
		m   discord.Member
		err error
	)

	m.ID, err = idFromNestedObject(buf, "user")
	if err != nil {
		return nil, xerrors.Errorf("extract user id: %w", err)
	}

	m.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	m.Raw = buf
	return &m, nil
}

func (_ decoder) DecodePresence(buf []byte) (*discord.Presence, error) {
	var (
		m   discord.Presence
		err error
	)

	m.ID, err = idFromNestedObject(buf, "user")
	if err != nil {
		return nil, xerrors.Errorf("extract user id: %w", err)
	}

	m.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	m.Raw = buf
	return &m, nil
}
