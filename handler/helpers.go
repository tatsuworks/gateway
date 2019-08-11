package handler

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"

	"github.com/tatsuworks/gateway/discordetf"
)

func (c *Client) HandleEvent(e *discordetf.Event) error {
	switch e.T {
	case "GUILD_CREATE":
		return c.GuildCreate(e.D)
	case "GUILD_UPDATE":
		return c.GuildCreate(e.D)
	case "GUILD_DELETE":
		return c.GuildDelete(e.D)
	case "GUILD_BAN_ADD":
		return c.GuildBanAdd(e.D)
	case "GUILD_BAN_REMOVE":
		return c.GuildBanRemove(e.D)
	case "GUILD_ROLE_CREATE":
		return c.RoleCreate(e.D)
	case "GUILD_ROLE_UPDATE":
		return c.RoleCreate(e.D)
	case "GUILD_ROLE_DELETE":
		return c.RoleDelete(e.D)
	case "GUILD_MEMBERS_CHUNK":
		return c.MemberChunk(e.D)
	case "GUILD_MEMBER_ADD":
		return c.MemberAdd(e.D)
	case "GUILD_MEMBER_UPDATE":
		return c.MemberAdd(e.D)
	case "GUILD_MEMBER_REMOVE":
		return c.MemberRemove(e.D)
	case "PRESENCE_UPDATE":
		// return c.PresenceUpdate(e.D)
		return nil
	case "CHANNEL_CREATE":
		return c.ChannelCreate(e.D)
	case "CHANNEL_UPDATE":
		return c.ChannelCreate(e.D)
	case "CHANNEL_DELETE":
		return c.ChannelDelete(e.D)
	case "VOICE_STATE_UPDATE":
		return c.VoiceStateUpdate(e.D)
	case "MESSAGE_CREATE":
		return c.MessageCreate(e.D)
	case "MESSAGE_UPDATE":
		return c.MessageCreate(e.D)
	case "MESSAGE_DELETE":
		return c.MessageDelete(e.D)
	case "MESSAGE_REACTION_ADD":
		return c.MessageReactionAdd(e.D)
	case "MESSAGE_REACTION_REMOVE":
		return c.MessageReactionRemove(e.D)
	case "MESSAGE_REACTION_REMOVE_ALL":
		return c.MessageReactionRemoveAll(e.D)
	case "TYPING_START":
		return nil
	case "nil":
		return nil
	default:
		// return errors.Errorf("unknown event: %s", e.T)
		return nil
	}
}

// Transact is a helper around (fdb.Database).Transact which accepts a function that doesn't require a return value.
func (c *Client) Transact(fn func(t fdb.Transaction) error) error {
	return c.db.Transact(fn)
}

// ReadTransact is a helper around (fdb.Database).ReadTransact which accepts a function that doesn't require a return value.
func (c *Client) ReadTransact(fn func(t fdb.ReadTransaction) error) error {
	return c.db.ReadTransact(fn)
}
