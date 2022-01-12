package handler

import (
	"bytes"
	"context"

	"github.com/tatsuworks/gateway/discord"
)

type EventPayload struct {
	GuildID    int64
	IsNewGuild bool
}

func (c *Client) HandleEvent(ctx context.Context, e *discord.Event) (*EventPayload, error) {
	// clear nulls
	e.D = bytes.ToValidUTF8(e.D, nil)

	switch e.T {
	case "PRESENCE_UPDATE":
		return nil, c.PresenceCreate(ctx, e.D)
	case "GUILD_CREATE":
		return c.GuildCreate(ctx, e.D)
	case "GUILD_UPDATE":
		_, err := c.GuildCreate(ctx, e.D)
		return nil, err
	case "GUILD_DELETE":
		return nil, c.GuildDelete(ctx, e.D)
	case "GUILD_BAN_ADD":
		return nil, c.GuildBanAdd(ctx, e.D)
	case "GUILD_BAN_REMOVE":
		return nil, c.GuildBanRemove(ctx, e.D)
	case "GUILD_ROLE_CREATE":
		return nil, c.RoleCreate(ctx, e.D)
	case "GUILD_ROLE_UPDATE":
		return nil, c.RoleCreate(ctx, e.D)
	case "GUILD_ROLE_DELETE":
		return nil, c.RoleDelete(ctx, e.D)
	case "GUILD_MEMBERS_CHUNK":
		return nil, c.MemberChunk(ctx, e.D)
	case "GUILD_MEMBER_ADD":
		return nil, c.MemberAdd(ctx, e.D)
	case "GUILD_MEMBER_UPDATE":
		return nil, c.MemberAdd(ctx, e.D)
	case "GUILD_MEMBER_REMOVE":
		return nil, c.MemberRemove(ctx, e.D)
	case "GUILD_EMOJIS_UPDATE":
		return nil, c.GuildEmojisUpdate(ctx, e.D)
	case "CHANNEL_CREATE":
		return nil, c.ChannelCreate(ctx, e.D)
	case "CHANNEL_UPDATE":
		return nil, c.ChannelCreate(ctx, e.D)
	case "CHANNEL_DELETE":
		return nil, c.ChannelDelete(ctx, e.D)
	case "VOICE_STATE_UPDATE":
		return nil, c.VoiceStateUpdate(ctx, e.D)
	case "THREAD_CREATE":
		return nil, c.ThreadCreate(ctx, e.D)
	case "THREAD_UPDATE":
		return nil, c.ThreadCreate(ctx, e.D)
	case "THREAD_DELETE":
		return nil, c.ThreadDelete(ctx, e.D)
	case "MESSAGE_CREATE":
		return nil, nil
	case "MESSAGE_UPDATE":
		return nil, nil
	case "MESSAGE_DELETE":
		return nil, nil
	case "MESSAGE_REACTION_ADD":
		return nil, nil
	case "MESSAGE_REACTION_REMOVE":
		return nil, nil
	case "MESSAGE_REACTION_REMOVE_ALL":
		return nil, nil
	case "TYPING_START":
		return nil, nil
	case "nil":
		return nil, nil
	default:
		return nil, nil
	}
}
