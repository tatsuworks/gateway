package handler

import (
	"sync/atomic"

	"cdr.dev/slog"
	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/internal/state"
)

type Client struct {
	log slog.Logger
	db  state.DB
	enc discord.Encoding

	// voiceGuilds are the guilds whose voice presence State keeps for voice
	// attendance. Every other guild's voice traffic is left as it was: not
	// stored. Nil means none.
	voiceGuilds map[int64]struct{}

	waitingQueries int64
}

// SetVoiceGuilds selects the guilds whose voice presence is stored.
func (c *Client) SetVoiceGuilds(guilds map[int64]struct{}) {
	c.voiceGuilds = guilds
}

// TracksVoice reports whether State keeps this guild's voice presence.
func (c *Client) TracksVoice(guild int64) bool {
	_, ok := c.voiceGuilds[guild]
	return ok
}

// VoiceStateError is a failed write of voice presence. The event's other
// handling succeeded, so the caller still publishes it, but State's voice
// cache for the guild can no longer be trusted.
type VoiceStateError struct {
	GuildID int64
	Err     error
}

func (e *VoiceStateError) Error() string { return "voice state: " + e.Err.Error() }
func (e *VoiceStateError) Unwrap() error { return e.Err }

func (c *Client) WaitingQueries() int64 {
	return atomic.LoadInt64(&c.waitingQueries)
}

func NewClient(log slog.Logger, db state.DB) *Client {
	return &Client{
		log: log,
		db:  db,
		enc: db.Encoding(),
	}
}
