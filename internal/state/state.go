package state

import "context"

type DB interface {
	SetGuild(ctx context.Context, id int64, raw []byte) error
	GetGuild(ctx context.Context, id int64) ([]byte, error)
	GetGuildCount(ctx context.Context) (int, error)
	DeleteGuild(ctx context.Context, id int64) error
	SetGuildBan(ctx context.Context, guild, user int64, raw []byte) error
	GetGuildBan(ctx context.Context, guild, user int64) ([]byte, error)
	DeleteGuildBan(ctx context.Context, guild, user int64) error

	SetChannel(ctx context.Context, guild, id int64, raw []byte) error
	GetChannel(ctx context.Context, id int64) ([]byte, error)
	GetChannelCount(ctx context.Context) (int, error)
	GetChannels(ctx context.Context) ([]map[int64][]byte, error)
	GetGuildChannels(ctx context.Context, guild int64) ([]map[int64][]byte, error)
	DeleteChannel(ctx context.Context, guild, id int64, raw []byte) error
	SetChannels(ctx context.Context, guild int64, channels map[int64][]byte) error
	DeleteChannels(ctx context.Context, guild int64) error
	SetVoiceState(ctx context.Context, guild, user int64, raw []byte) error

	SetGuildMembers(ctx context.Context, guild int64, raws map[int64][]byte) error
	DeleteGuildMembers(ctx context.Context, guild int64) error
	SetGuildMember(ctx context.Context, guild, user int64, raw []byte) error
	GetGuildMember(ctx context.Context, guild, user int64) ([]byte, error)
	GetGuildMembers(ctx context.Context, guild int64) ([]map[int64][]byte, error)
	DeleteGuildMember(ctx context.Context, guild, user int64) error

	SetChannelMessage(ctx context.Context, channel, id int64, raw []byte) error
	GetChannelMessage(ctx context.Context, channel, id int64) ([]byte, error)
	DeleteChannelMessage(ctx context.Context, channel, id int64) error
	SetChannelMessageReaction(ctx context.Context, channel, id, user int64, name interface{}, raw []byte) error
	DeleteChannelMessageReaction(ctx context.Context, channel, id, user int64, name interface{}) error
	DeleteChannelMessageReactions(ctx context.Context, channel, id, user int64) error

	SetGuildRole(ctx context.Context, guild, role int64, raw []byte) error
	GetGuildRole(ctx context.Context, guild, role int64) ([]byte, error)
	SetGuildRoles(ctx context.Context, guild int64, roles map[int64][]byte) error
	GetGuildRoles(ctx context.Context, guild int64) ([]map[int64][]byte, error)
	DeleteGuildRoles(ctx context.Context, guild int64) error
	DeleteGuildRole(ctx context.Context, guild, role int64) error
}
