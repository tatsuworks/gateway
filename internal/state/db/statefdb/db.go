package statefdb

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/directory"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/discord/discordetf"
	"github.com/tatsuworks/gateway/internal/state"
)

type DB struct {
	fdb  fdb.Database
	subs *Subspaces
}

func NewDB() (state.DB, error) {
	fdb.MustAPIVersion(610)
	db := fdb.MustOpenDefault()

	dir, err := directory.CreateOrOpen(db, []string{"state"}, nil)
	if err != nil {
		return nil, xerrors.Errorf("create fdb directory: %w", err)
	}

	return &DB{
		subs: NewSubspaces(dir),
		fdb:  db,
	}, nil
}

func (db *DB) Encoding() discord.Encoding {
	return discordetf.Encoding
}

// Transact is a helper around (fdb.Database).Transact which accepts a function
// that doesn't require a return value.
func (db *DB) Transact(fn func(t fdb.Transaction) error) error {
	_, err := db.fdb.Transact(func(t fdb.Transaction) (ret interface{}, err error) {
		return nil, fn(t)
	})
	if err != nil {
		return xerrors.Errorf("commit fdb txn: %w", err)

	}

	return nil
}

// ReadTransact is a helper around (fdb.Database).ReadTransact which accepts a
// function that doesn't require a return value.
func (db *DB) ReadTransact(fn func(t fdb.ReadTransaction) error) error {
	_, err := db.fdb.ReadTransact(func(t fdb.ReadTransaction) (ret interface{}, err error) {
		return nil, fn(t)
	})
	if err != nil {
		return xerrors.Errorf("commit fdb read txn: %w", err)
	}

	return nil
}

func (db *DB) fmtChannelKey(id int64) fdb.Key {
	return db.subs.Channels.Pack(tuple.Tuple{id})
}

func (db *DB) fmtChannelPrefix() fdb.Key {
	return db.subs.Channels.FDBKey()
}

func (db *DB) fmtGuildChannelKey(guild, id int64) fdb.Key {
	return db.subs.Channels.Pack(tuple.Tuple{guild, id})
}

func (db *DB) fmtGuildChannelPrefix(guild int64) fdb.Key {
	return db.subs.Channels.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtGuildKey(guild int64) fdb.Key {
	return db.subs.Guilds.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtGuildPrefix() fdb.Key {
	return db.subs.Guilds.FDBKey()
}

func (db *DB) fmtGuildBanKey(guild, user int64) fdb.Key {
	return db.subs.Guilds.Pack(tuple.Tuple{guild, "bans", user})
}

func (db *DB) fmtGuildMemberKey(guild, id int64) fdb.Key {
	return db.subs.Members.Pack(tuple.Tuple{guild, id})
}

func (db *DB) fmtGuildMemberPrefix(guild int64) fdb.Key {
	return db.subs.Members.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtMemberPrefix() fdb.Key {
	return db.subs.Members.FDBKey()
}

func (db *DB) fmtChannelMessageKey(channel, id int64) fdb.Key {
	return db.subs.Messages.Pack(tuple.Tuple{channel, id})
}

func (db *DB) fmtChannelMessagePrefix(channel int64) fdb.Key {
	return db.subs.Messages.Pack(tuple.Tuple{channel})
}

func (db *DB) fmtMessagePrefix() fdb.Key {
	return db.subs.Messages.FDBKey()
}

func (db *DB) fmtMessageReactionKey(channel, id, user int64, name interface{}) fdb.Key {
	return db.subs.Messages.Pack(tuple.Tuple{channel, id, "rxns", user, name})
}

func (db *DB) fmtMessageReactionPrefix(channel, id, user int64) fdb.Key {
	return db.subs.Messages.Pack(tuple.Tuple{channel, id, "rxns", user})
}

func (db *DB) fmtGuildPresenceKey(guild, id int64) fdb.Key {
	return db.subs.Presences.Pack(tuple.Tuple{guild, id})
}

func (db *DB) fmtGuildPresencePrefix(guild int64) fdb.Key {
	return db.subs.Presences.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtPresencePrefix() fdb.Key {
	return db.subs.Presences.FDBKey()
}

func (db *DB) fmtGuildRoleKey(guild, id int64) fdb.Key {
	return db.subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (db *DB) fmtGuildRolePrefix(guild int64) fdb.Key {
	return db.subs.Roles.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtRolePrefix() fdb.Key {
	return db.subs.Roles.FDBKey()
}

func (db *DB) fmtGuildVoiceStateKey(guild, user int64) fdb.Key {
	return db.subs.VoiceStates.Pack(tuple.Tuple{guild, user})
}

func (db *DB) fmtGuildVoiceStatePrefix(guild int64) fdb.Key {
	return db.subs.VoiceStates.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtVoiceStatePrefix() fdb.Key {
	return db.subs.VoiceStates.FDBKey()
}

func (db *DB) fmtGuildEmojiKey(guild int64, emoji int64) fdb.Key {
	return db.subs.Emojis.Pack(tuple.Tuple{guild, emoji})
}

func (db *DB) fmtGuildEmojiPrefix(guild int64) fdb.Key {
	return db.subs.Emojis.Pack(tuple.Tuple{guild})
}

func (db *DB) fmtEmojiPrefix(guild int64, emoji int64) fdb.Key {
	return db.subs.Emojis.FDBKey()
}
