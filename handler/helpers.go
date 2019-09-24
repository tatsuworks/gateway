package handler

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"

	"github.com/tatsuworks/gateway/discordetf"
)

func (c *Client) HandleEvent(e *discordetf.Event) (int64, error) {
	switch e.T {
	case "GUILD_CREATE":
		return c.GuildCreate(e.D)
	case "GUILD_UPDATE":
		_, err := c.GuildCreate(e.D)
		return 0, err
	case "GUILD_DELETE":
		return 0, c.GuildDelete(e.D)
	case "GUILD_BAN_ADD":
		return 0, c.GuildBanAdd(e.D)
	case "GUILD_BAN_REMOVE":
		return 0, c.GuildBanRemove(e.D)
	case "GUILD_ROLE_CREATE":
		return 0, c.RoleCreate(e.D)
	case "GUILD_ROLE_UPDATE":
		return 0, c.RoleCreate(e.D)
	case "GUILD_ROLE_DELETE":
		return 0, c.RoleDelete(e.D)
	case "GUILD_MEMBERS_CHUNK":
		return 0, c.MemberChunk(e.D)
	case "GUILD_MEMBER_ADD":
		return 0, c.MemberAdd(e.D)
	case "GUILD_MEMBER_UPDATE":
		return 0, c.MemberAdd(e.D)
	case "GUILD_MEMBER_REMOVE":
		return 0, c.MemberRemove(e.D)
	case "PRESENCE_UPDATE":
		// return 0, c.PresenceUpdate(e.D)
		return 0, nil
	case "CHANNEL_CREATE":
		return 0, c.ChannelCreate(e.D)
	case "CHANNEL_UPDATE":
		return 0, c.ChannelCreate(e.D)
	case "CHANNEL_DELETE":
		return 0, c.ChannelDelete(e.D)
	case "VOICE_STATE_UPDATE":
		return 0, c.VoiceStateUpdate(e.D)
	case "MESSAGE_CREATE":
		// return 0, c.MessageCreate(e.D)
		return 0, nil
	case "MESSAGE_UPDATE":
		return 0, nil
		// return 0, c.MessageCreate(e.D)
	case "MESSAGE_DELETE":
		return 0, nil
		// return 0, c.MessageDelete(e.D)
	case "MESSAGE_REACTION_ADD":
		return 0, nil
		// return 0, c.MessageReactionAdd(e.D)
	case "MESSAGE_REACTION_REMOVE":
		return 0, nil
		// return 0, c.MessageReactionRemove(e.D)
	case "MESSAGE_REACTION_REMOVE_ALL":
		return 0, nil
		// return 0, c.MessageReactionRemoveAll(e.D)
	case "TYPING_START":
		return 0, nil
	case "nil":
		return 0, nil
	default:
		// return 0, errors.Errorf("unknown event: %s", e.T)
		return 0, nil
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
