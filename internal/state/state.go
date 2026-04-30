package state

import (
	"context"

	"github.com/tatsuworks/gateway/discord"
)

type UserAndData struct {
	UserID   string `db:"id"`
	Username string `db:"username"`
}

type DB interface {
	Encoding() discord.Encoding

	GetShardInfo(ctx context.Context, shard int, name string) (sess string, seq int64, err error)
	SetShardInfo(ctx context.Context, shard int, name string, seq int64, sess string, resumeURL string) error
	SetSequence(ctx context.Context, shard int, name string, seq int64) error
	GetSequence(ctx context.Context, shard int, name string) (int64, error)
	SetSessionID(ctx context.Context, shard int, name string, sess string) error
	GetSessionID(ctx context.Context, shard int, name string) (string, error)
	SetStatus(ctx context.Context, shard int, name string, status string) error
	// CountUnstableShards returns how many shards in [start, stop) for
	// this name are currently outside the steady-state inner-loop
	// (`read message` / `push event to redis`). Used as an instability
	// proxy during deploys and mass reconnects — a healthy fleet should
	// trend toward zero. Status is updated by Session.logTotalEvents at
	// 1-minute resolution, so the count is up to ~1 minute stale.
	CountUnstableShards(ctx context.Context, name string, start, stop int) (int, error)
	SetResumeGatewayURL(ctx context.Context, shard int, name string, resumeURL string) error
	GetResumeGatewayURL(ctx context.Context, shard int, name string) (string, error)

	SetGuild(ctx context.Context, id int64, raw []byte) error
	GetGuild(ctx context.Context, id int64) ([]byte, error)
	GetGuildCount(ctx context.Context) (int, error)
	// GetGuildIDsAfter pages through guild IDs in ascending order,
	// returning IDs strictly greater than `after`, capped at `limit`.
	// Used by the bounded-rate background member-resync sweep so we can
	// iterate the guild set without materializing it all in memory.
	// Returns an empty slice (not an error) when no IDs are above the
	// cursor — callers wrap to `after = 0`.
	GetGuildIDsAfter(ctx context.Context, after int64, limit int) ([]int64, error)
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
	DeleteChannelsById(ctx context.Context, guild int64, channelIDs []int64) error

	SetGuildMembers(ctx context.Context, guild int64, raws map[int64][]byte) error
	DeleteGuildMembers(ctx context.Context, guild int64) error
	SetGuildMember(ctx context.Context, guild, user int64, raw []byte, isNew bool) error
	GetGuildMember(ctx context.Context, guild, user int64) ([]byte, error)
	GetGuildMemberCount(ctx context.Context, guild int64) (int, error)
	GetGuildMembers(ctx context.Context, guild int64) ([][]byte, error)
	GetGuildMembersWithRole(ctx context.Context, guild, role int64) ([][]byte, error)
	DeleteGuildMember(ctx context.Context, guild, user int64) error
	SearchGuildMembers(ctx context.Context, guildID int64, query string, limit int) ([][]byte, error)
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
	DeleteGuildRolesById(ctx context.Context, guildID int64, roleIDs []int64) error
	ExistUserInGuildsHasRoles(ctx context.Context, guildIDs []int64, roleIDs []string, userID int64) (bool, error)
	ExistUserInGuilds(ctx context.Context, guildIDs []int64, userID int64) (bool, error)

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
