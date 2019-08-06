package handler

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

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
		return c.PresenceUpdate(e.D)
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
	_, err := c.fdb.Transact(func(t fdb.Transaction) (ret interface{}, err error) {
		return nil, fn(t)
	})

	return errors.Wrap(err, "failed to commit fdb txn")
}

// ReadTransact is a helper around (fdb.Database).ReadTransact which accepts a function that doesn't require a return value.
func (c *Client) ReadTransact(fn func(t fdb.ReadTransaction) error) error {
	_, err := c.fdb.ReadTransact(func(t fdb.ReadTransaction) (ret interface{}, err error) {
		return nil, fn(t)
	})

	return errors.Wrap(err, "failed to commit fdb read txn")
}

func (c *Client) setETFs(guild int64, etfs map[int64][]byte, key func(id int64) fdb.Key) error {
	eg := new(errgroup.Group)

	send := func(guild int64, etfs map[int64][]byte, key func(id int64) fdb.Key) {
		eg.Go(func() error {
			return c.Transact(func(t fdb.Transaction) error {
				opts := t.Options()
				opts.SetReadYourWritesDisable()

				for id, e := range etfs {
					opts.SetNextWriteNoWriteConflictRange()
					t.Set(key(id), e)
				}

				return nil
			})
		})
	}

	bufMap := etfs

	// FDB recommends 10KB per transaction. If we limit transactions to
	// 100 keys each, we allow an average of 100 bytes per k/v pair.
	if len(etfs) > 100 {
		bufMap = make(map[int64][]byte, 100)

		for i, e := range etfs {
			bufMap[i] = e

			if len(bufMap) >= 100 {
				send(guild, bufMap, key)
				bufMap = make(map[int64][]byte, 100)
			}
		}
	}

	send(guild, bufMap, key)
	return eg.Wait()
}

func (c *Client) setGuildETFs(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) error {
	eg := new(errgroup.Group)

	send := func(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) {
		eg.Go(func() error {
			return c.Transact(func(t fdb.Transaction) error {
				opts := t.Options()
				opts.SetReadYourWritesDisable()

				for id, e := range etfs {
					opts.SetNextWriteNoWriteConflictRange()
					t.Set(key(guild, id), e)
				}

				return nil
			})
		})
	}

	bufMap := etfs

	// FDB recommends 10KB per transaction. If we limit transactions to
	// 100 keys each, we allow an average of 100 bytes per k/v pair.
	if len(etfs) > 100 {
		bufMap = make(map[int64][]byte, 100)

		for i, e := range etfs {
			bufMap[i] = e

			if len(bufMap) >= 100 {
				send(guild, bufMap, key)
				bufMap = make(map[int64][]byte, 100)
			}
		}
	}

	send(guild, bufMap, key)
	return eg.Wait()
}

func (c *Client) fmtChannelKey(id int64) fdb.Key {
	return c.subs.Channels.Pack(tuple.Tuple{id})
}

func (c *Client) fmtChannelPrefix() fdb.Key {
	return c.subs.Channels.FDBKey()
}

func (c *Client) fmtGuildChannelKey(guild, id int64) fdb.Key {
	return c.subs.Channels.Pack(tuple.Tuple{guild, id})
}

func (c *Client) fmtGuildChannelPrefix(guild int64) fdb.Key {
	return c.subs.Channels.Pack(tuple.Tuple{guild})
}

func (c *Client) fmtGuildKey(guild int64) fdb.Key {
	return c.subs.Guilds.Pack(tuple.Tuple{guild})
}

func (c *Client) fmtGuildPrefix() fdb.Key {
	return c.subs.Guilds.FDBKey()
}

func (c *Client) fmtGuildBanKey(guild, user int64) fdb.Key {
	return c.subs.Guilds.Pack(tuple.Tuple{guild, "bans", user})
}

func (c *Client) fmtGuildMemberKey(guild, id int64) fdb.Key {
	return c.subs.Members.Pack(tuple.Tuple{guild, id})
}

func (c *Client) fmtGuildMemberPrefix(guild int64) fdb.Key {
	return c.subs.Members.Pack(tuple.Tuple{guild})
}

func (c *Client) fmtMemberPrefix() fdb.Key {
	return c.subs.Members.FDBKey()
}

func (c *Client) fmtChannelMessageKey(channel, id int64) fdb.Key {
	return c.subs.Messages.Pack(tuple.Tuple{channel, id})
}

func (c *Client) fmtChannelMessagePrefix(channel int64) fdb.Key {
	return c.subs.Messages.Pack(tuple.Tuple{channel})
}

func (c *Client) fmtMessagePrefix() fdb.Key {
	return c.subs.Messages.FDBKey()
}

func (c *Client) fmtMessageReactionKey(channel, id, user int64, name interface{}) fdb.Key {
	return c.subs.Messages.Pack(tuple.Tuple{channel, id, "rxns", user, name})
}

func (c *Client) fmtGuildPresenceKey(guild, id int64) fdb.Key {
	return c.subs.Presences.Pack(tuple.Tuple{guild, id})
}

func (c *Client) fmtGuildPresencePrefix(guild int64) fdb.Key {
	return c.subs.Presences.Pack(tuple.Tuple{guild})
}

func (c *Client) fmtPresencePrefix() fdb.Key {
	return c.subs.Presences.FDBKey()
}

func (c *Client) fmtGuildRoleKey(guild, id int64) fdb.Key {
	return c.subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (c *Client) fmtGuildRolePrefix(guild int64) fdb.Key {
	return c.subs.Roles.Pack(tuple.Tuple{guild})
}

func (c *Client) fmtRolePrefix() fdb.Key {
	return c.subs.Roles.FDBKey()
}

func (c *Client) fmtGuildVoiceStateKey(guild, user int64) fdb.Key {
	return c.subs.VoiceStates.Pack(tuple.Tuple{guild, user})
}

func (c *Client) fmtGuildVoiceStatePrefix(guild int64) fdb.Key {
	return c.subs.VoiceStates.Pack(tuple.Tuple{guild})
}

func (c *Client) fmtVoiceStatePrefix() fdb.Key {
	return c.subs.VoiceStates.FDBKey()
}
