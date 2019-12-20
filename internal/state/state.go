package state

type DB interface {
	SetGuild(id int64, raw []byte) error
	GetGuild(id int64) ([]byte, error)
	GetGuildCount() (int, error)
	DeleteGuild(id int64) error
	SetGuildBan(guild, user int64, raw []byte) error
	GetGuildBan(guild, user int64) ([]byte, error)
	DeleteGuildBan(guild, user int64) error

	SetChannel(guild, id int64, raw []byte) error
	GetChannel(id int64) ([]byte, error)
	GetChannelCount() (int, error)
	GetChannels() ([]map[int64][]byte, error)
	GetGuildChannels(guild int64) ([]map[int64][]byte, error)
	DeleteChannel(guild, id int64, raw []byte) error
	SetChannels(guild int64, channels map[int64][]byte) error
	DeleteChannels(guild int64) error
	SetVoiceState(guild, user int64, raw []byte) error

	SetGuildMembers(guild int64, raws map[int64][]byte) error
	DeleteGuildMembers(guild int64) error
	SetGuildMember(guild, user int64, raw []byte) error
	GetGuildMember(guild, user int64) ([]byte, error)
	GetGuildMembers(guild int64) ([]map[int64][]byte, error)
	DeleteGuildMember(guild, user int64) error

	SetChannelMessage(channel, id int64, raw []byte) error
	GetChannelMessage(channel, id int64) ([]byte, error)
	DeleteChannelMessage(channel, id int64) error
	SetChannelMessageReaction(channel, id, user int64, name interface{}, raw []byte) error
	DeleteChannelMessageReaction(channel, id, user int64, name interface{}) error
	DeleteChannelMessageReactions(channel, id, user int64) error

	SetGuildRole(guild, role int64, raw []byte) error
	GetGuildRole(guild, role int64) ([]byte, error)
	SetGuildRoles(guild int64, roles map[int64][]byte) error
	GetGuildRoles(guild int64) ([]map[int64][]byte, error)
	DeleteGuildRoles(guild int64) error
	DeleteGuildRole(guild, role int64) error
}
