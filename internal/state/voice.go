package state

import "time"

// VoiceOccupant is one account's current voice location as State last saw it.
type VoiceOccupant struct {
	UserID    int64
	ChannelID int64
	// IsBot is nil when the account could not be classified.
	IsBot      *bool
	ChangedAt  time.Time
	DiscordSeq int64
}

// VoiceStateChange is one received VOICE_STATE_UPDATE. ChannelID 0 is a
// departure. ChangedAt and DiscordSeq come from the gateway receipt.
type VoiceStateChange struct {
	GuildID    int64
	UserID     int64
	ChannelID  int64
	IsBot      *bool
	ChangedAt  time.Time
	DiscordSeq int64
	Raw        []byte
}

// VoiceBaseline is a guild's complete voice presence from one GUILD_CREATE.
// It replaces whatever State held for the guild and marks the guild ready on
// that connection.
type VoiceBaseline struct {
	GuildID      int64
	ShardID      int
	ConnectionID string
	ReceivedAt   time.Time
	DiscordSeq   int64
	Occupants    []VoiceOccupant
	// Raws holds each occupant's raw voice state, keyed by user ID.
	Raws map[int64][]byte
}

// VoiceLoss is a recorded real loss of voice observation on a shard.
type VoiceLoss struct {
	Revision     int64
	ShardID      int
	Reason       string
	KnownThrough time.Time
	DetectedAt   time.Time
}

// VoiceSnapshot is a guild's current voice presence together with the
// evidence that it is trustworthy, read in one consistent snapshot.
type VoiceSnapshot struct {
	GuildID int64
	// ShardID is -1 when State holds no readiness for the guild.
	ShardID      int
	ConnectionID string
	// ObservedAt is the database time the snapshot was read.
	ObservedAt time.Time
	// Ready means a complete baseline on the shard's current, heartbeating
	// connection. Complete additionally means every occupant is classified.
	Ready          bool
	Complete       bool
	NotReadyReason string
	Occupants      []VoiceOccupant
	// Loss is the most recent loss on the shard after the caller's revision,
	// with KnownThrough lowered to the earliest such loss so a later failure
	// never hides an earlier cutoff. nil when there is none.
	Loss *VoiceLoss
}
