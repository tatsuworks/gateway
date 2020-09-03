package handler

import (
	"bytes"
	"context"

	"github.com/tatsuworks/gateway/discord"
)

func (c *Client) HandleEvent(ctx context.Context, e *discord.Event) (int64, error) {
	// clear nulls
	e.D = bytes.ToValidUTF8(e.D, nil)

	switch e.T {
	case "PRESENCE_UPDATE":
		// return 0, c.PresenceUpdate(e.D)
		return 0, nil
	case "GUILD_CREATE":
		return c.GuildCreate(ctx, e.D)
	case "GUILD_UPDATE":
		_, err := c.GuildCreate(ctx, e.D)
		return 0, err
	case "GUILD_DELETE":
		return 0, c.GuildDelete(ctx, e.D)
	case "GUILD_BAN_ADD":
		return 0, c.GuildBanAdd(ctx, e.D)
	case "GUILD_BAN_REMOVE":
		return 0, c.GuildBanRemove(ctx, e.D)
	case "GUILD_ROLE_CREATE":
		return 0, c.RoleCreate(ctx, e.D)
	case "GUILD_ROLE_UPDATE":
		return 0, c.RoleCreate(ctx, e.D)
	case "GUILD_ROLE_DELETE":
		return 0, c.RoleDelete(ctx, e.D)
	case "GUILD_MEMBERS_CHUNK":
		return 0, c.MemberChunk(ctx, e.D)
	case "GUILD_MEMBER_ADD":
		return 0, c.MemberAdd(ctx, e.D)
	case "GUILD_MEMBER_UPDATE":
		return 0, c.MemberAdd(ctx, e.D)
	case "GUILD_MEMBER_REMOVE":
		return 0, c.MemberRemove(ctx, e.D)
	case "GUILD_EMOJIS_UPDATE":
		return 0, c.GuildEmojisUpdate(ctx, e.D)
	case "CHANNEL_CREATE":
		return 0, c.ChannelCreate(ctx, e.D)
	case "CHANNEL_UPDATE":
		return 0, c.ChannelCreate(ctx, e.D)
	case "CHANNEL_DELETE":
		return 0, c.ChannelDelete(ctx, e.D)
	case "VOICE_STATE_UPDATE":
		return 0, c.VoiceStateUpdate(ctx, e.D)
	case "MESSAGE_CREATE":
		return 0, nil
	case "MESSAGE_UPDATE":
		return 0, nil
	case "MESSAGE_DELETE":
		return 0, nil
	case "MESSAGE_REACTION_ADD":
		return 0, nil
	case "MESSAGE_REACTION_REMOVE":
		return 0, nil
	case "MESSAGE_REACTION_REMOVE_ALL":
		return 0, nil
	case "TYPING_START":
		return 0, nil
	case "nil":
		return 0, nil
	default:
		return 0, nil
	}
}
