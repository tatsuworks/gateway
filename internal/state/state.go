package state

import (
	"context"

	"github.com/tatsuworks/gateway/discord"
)

type UserAndData struct {
    UserID string  `db:"id"`
    User   string `db:"user"`
}

type DB interface {
	Encoding() discord.Encoding

	GetShardInfo(ctx context.Context, shard int, name string) (sess string, seq int64, err error)
	SetSequence(ctx context.Context, shard int, name string, seq int64) error
	GetSequence(ctx context.Context, shard int, name string) (int64, error)
	SetSessionID(ctx context.Context, shard int, name string, sess string) error
	GetSessionID(ctx context.Context, shard int, name string) (string, error)
	SetStatus(ctx context.Context, shard int, name string, status string) error
	SetResumeGatewayURL(ctx context.Context, shard int, name string, resumeURL string) error
	GetResumeGatewayURL(ctx context.Context, shard int, name string) (string, error)

	SetGuild(ctx context.Context, id int64, raw []byte) (bool, error)
	GetGuild(ctx context.Context, id int64) ([]byte, error)
	GetGuildCount(ctx context.Context) (int, error)
	DeleteGuild(ctx context.Context, id int64) error
	SetGuildBan(ctx context.Context, guild, user int64, raw []byte) error
	GetGuildBan(ctx context.Context, guild, user int64) ([]byte, error)
	DeleteGuildBan(ctx context.Context, guild, user int64) error

	SetChannel(ctx context.Context, guild, id int64, raw []byte) error
	GetChannel(ctx context.Context, id int64) ([]byte, error)
	GetChannelCount(ctx context.Context) (int, error)
	GetChannels(ctx context.Context) ([][]byte, error)
	GetGuildChannels(ctx context.Context, guild int64) ([][]byte, error)
	DeleteChannel(ctx context.Context, guild, id int64) error
	SetChannels(ctx context.Context, guild int64, channels map[int64][]byte) error
	DeleteChannels(ctx context.Context, guild int64) error
	SetVoiceState(ctx context.Context, guild, user int64, raw []byte) error

	SetGuildMembers(ctx context.Context, guild int64, raws map[int64][]byte) error
	DeleteGuildMembers(ctx context.Context, guild int64) error
	SetGuildMember(ctx context.Context, guild, user int64, raw []byte) error
	GetGuildMember(ctx context.Context, guild, user int64) ([]byte, error)
	GetGuildMemberCount(ctx context.Context, guild int64) (int, error)
	GetGuildMembers(ctx context.Context, guild int64) ([][]byte, error)
	GetGuildMembersWithRole(ctx context.Context, guild, role int64) ([][]byte, error)
	DeleteGuildMember(ctx context.Context, guild, user int64) error
	SearchGuildMembers(ctx context.Context, guildID int64, query string) ([][]byte, error)
	SetPresence(ctx context.Context, guild, user int64, raw []byte) error
	GetUserPresence(ctx context.Context, guildID, userID int64) ([]byte, error)
	SetPresences(ctx context.Context, guildID int64, presences map[int64][]byte) error

	SetChannelMessage(ctx context.Context, channel, id int64, raw []byte) error
	GetChannelMessage(ctx context.Context, channel, id int64) ([]byte, error)
	DeleteChannelMessage(ctx context.Context, channel, id int64) error
	SetChannelMessageReaction(ctx context.Context, channel, id, user int64, name interface{}, raw []byte) error
	DeleteChannelMessageReaction(ctx context.Context, channel, id, user int64, name interface{}) error
	DeleteChannelMessageReactions(ctx context.Context, channel, id, user int64) error

	SetGuildRole(ctx context.Context, guild, role int64, raw []byte) error
	GetGuildRole(ctx context.Context, guild, role int64) ([]byte, error)
	SetGuildRoles(ctx context.Context, guild int64, roles map[int64][]byte) error
	GetGuildRoles(ctx context.Context, guild int64) ([][]byte, error)
	DeleteGuildRoles(ctx context.Context, guild int64) error
	DeleteGuildRole(ctx context.Context, guild, role int64) error

	SetGuildEmojis(ctx context.Context, guild int64, raws map[int64][]byte) error
	SetGuildEmoji(ctx context.Context, guild, emoji int64, raw []byte) error
	GetGuildEmoji(ctx context.Context, guild, emoji int64) ([]byte, error)
	GetGuildEmojis(ctx context.Context, guild int64) ([][]byte, error)
	DeleteGuildEmoji(ctx context.Context, guild, emoji int64) error

	GetUser(ctx context.Context, userID int64) ([]byte, error)
	GetUsersDiscordIdAndUsername(ctx context.Context, userIDs []int64) ([]UserAndData, error)

	SetThreads(ctx context.Context, guild int64, threads map[int64][]byte) error
	SetThread(ctx context.Context, guild, parent, owner, id int64, raw []byte) error
	GetThread(ctx context.Context, id int64) ([]byte, error)
	GetThreadsCount(ctx context.Context) (int, error)
	GetThreads(ctx context.Context) ([][]byte, error)
	GetGuildThreads(ctx context.Context, guild int64) ([][]byte, error)
	GetChannelThreads(ctx context.Context, channel int64) ([][]byte, error)
	DeleteThread(ctx context.Context, id int64) error
	DeleteThreads(ctx context.Context, guild int64) error
}
