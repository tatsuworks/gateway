package discord

type Event struct {
	D  []byte
	Op int
	S  int64
	T  string
}

type GuildCreate struct {
	ID          int64
	Raw         []byte
	MemberCount int64
	Channels    map[int64][]byte
	Threads     map[int64][]byte
	Emojis      map[int64][]byte
	Members     map[int64][]byte
	Presences   map[int64][]byte
	Roles       map[int64][]byte
	VoiceStates map[int64][]byte
}

type GuildBan struct {
	UserID  int64
	GuildID int64
	Raw     []byte
}

type Channel struct {
	ID      int64
	GuildID int64
	Raw     []byte
}

type Thread struct {
	ID       int64
	OwnerID  int64 // user who started the thread
	ParentID int64
	GuildID  int64
	Raw      []byte
}

type VoiceState struct {
	UserID  int64
	GuildID int64
	Raw     []byte
}

type MemberChunk struct {
	GuildID int64
	Members map[int64][]byte
}

type Member struct {
	ID      int64
	GuildID int64
	Raw     []byte
}

type Presence struct {
	ID      int64
	GuildID int64
	Raw     []byte
}

type PlayedPresence struct {
	UserID int64
	Game   string
}

type Message struct {
	ID        int64
	ChannelID int64
	Raw       []byte
}

type MessageReaction struct {
	MessageID int64
	ChannelID int64
	UserID    int64
	Name      interface{}
	Raw       []byte
}

type MessageReactionRemoveAll struct {
	MessageID int64
	ChannelID int64
	UserID    int64
}

type Role struct {
	ID      int64
	GuildID int64
	Raw     []byte
}

type RoleDelete struct {
	ID      int64
	GuildID int64
}

type GuildEmojisUpdate struct {
	GuildID int64
	Emojis  map[int64][]byte
}
