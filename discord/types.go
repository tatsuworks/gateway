package discord

import "time"

type Event struct {
	D  []byte
	Op int
	S  int64
	T  string

	// Receipt describes when and on which connection the gateway received a
	// dispatch. It is stamped right after decode, before state handling and
	// Redis publication, and is never part of the published payload itself.
	Receipt Receipt
}

// Receipt is the gateway's own record of receiving one dispatch. A zero value
// means the event did not come off a live connection (tests, synthetic events).
type Receipt struct {
	ShardID int
	// ConnectionID identifies the Discord session that delivered the event. A
	// successful RESUME keeps it; a new IDENTIFY changes it.
	ConnectionID string
	ReceivedAt   time.Time
}

type GuildCreate struct {
	ID int64
	// Unavailable is Discord's outage marker. An unavailable guild carries no
	// state, so it must never be read as an empty guild.
	Unavailable bool
	Raw         []byte
	MemberCount int64
	Channels    map[int64][]byte
	Threads     map[int64][]byte
	Emojis      map[int64][]byte
	Members     map[int64][]byte
	Presences   map[int64][]byte
	Roles       map[int64][]byte
	VoiceStates map[int64][]byte
	// VoiceStatesPresent distinguishes a payload that listed voice states (an
	// empty list means nobody is connected) from one that never carried the
	// key, such as GUILD_UPDATE.
	VoiceStatesPresent bool
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
	// ChannelID is 0 when the user left voice.
	ChannelID int64
	// IsBot is nil when the payload did not carry the member's user object, so
	// the account cannot be classified from this event alone.
	IsBot *bool
	Raw   []byte
}

type MemberChunk struct {
	GuildID    int64
	Members    map[int64][]byte
	ChunkIndex int
	ChunkCount int
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
