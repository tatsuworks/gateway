package discordjson

import (
	"github.com/tatsuworks/gateway/discord"
	"golang.org/x/xerrors"
)

func (_ decoder) DecodeThread(buf []byte) (*discord.Thread, error) {
	id, err := snowflakeFromObject(buf, "id")
	if err != nil {
		return nil, xerrors.Errorf("extract id from thread: %w", err)
	}

	ownerID, err := snowflakeFromObject(buf, "owner_id") // user who start the thread
	if err != nil {
		return nil, xerrors.Errorf("extract owner id: %w", err)
	}

	parentID, err := snowflakeFromObject(buf, "parent_id") //  id of the GUILD_TEXT or GUILD_NEWS channel the thread was created in
	if err != nil {
		return nil, xerrors.Errorf("extract parent id: %w", err)
	}

	guildID, err := snowflakeFromObjectOptional(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild_id from thread: %w", err)
	}

	return &discord.Thread{
		ID:       id,
		OwnerID:  ownerID,
		ParentID: parentID,
		GuildID:  guildID,
		Raw:      buf,
	}, nil
}
