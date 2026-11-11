package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/discord/discordjson"
	"github.com/tatsuworks/gateway/internal/state"
)

// voiceFakeDB records the voice calls and accepts the guild writes
// GUILD_CREATE makes. Any other state.DB method panics through the nil
// embedded interface, which flags an unexpected call.
type voiceFakeDB struct {
	state.DB

	applied   []state.VoiceStateChange
	baselines []state.VoiceBaseline
	cleared   []int64
	channels  [][2]int64
	fail      error
}

func (f *voiceFakeDB) Encoding() discord.Encoding { return discordjson.Encoding }
func (f *voiceFakeDB) ApplyVoiceState(_ context.Context, c state.VoiceStateChange) error {
	f.applied = append(f.applied, c)
	return f.fail
}
func (f *voiceFakeDB) ReplaceGuildVoiceStates(_ context.Context, b state.VoiceBaseline) error {
	f.baselines = append(f.baselines, b)
	return f.fail
}
func (f *voiceFakeDB) ClearGuildVoiceStates(_ context.Context, g int64) error {
	f.cleared = append(f.cleared, g)
	return f.fail
}
func (f *voiceFakeDB) DeleteChannelVoiceStates(_ context.Context, g, c int64) error {
	f.channels = append(f.channels, [2]int64{g, c})
	return f.fail
}
func (f *voiceFakeDB) SetGuild(context.Context, int64, []byte) error                  { return nil }
func (f *voiceFakeDB) SetGuildRoles(context.Context, int64, map[int64][]byte) error   { return nil }
func (f *voiceFakeDB) SetGuildMembers(context.Context, int64, map[int64][]byte) error { return nil }
func (f *voiceFakeDB) SetChannels(context.Context, int64, map[int64][]byte) error     { return nil }
func (f *voiceFakeDB) SetGuildEmojis(context.Context, int64, map[int64][]byte) error  { return nil }
func (f *voiceFakeDB) SetThreads(context.Context, int64, map[int64][]byte) error      { return nil }
func (f *voiceFakeDB) SetPresences(context.Context, int64, map[int64][]byte) error    { return nil }
func (f *voiceFakeDB) GetGuildMemberCount(context.Context, int64) (int, error)        { return 0, nil }
func (f *voiceFakeDB) DeleteGuild(context.Context, int64) error                       { return nil }
func (f *voiceFakeDB) DeleteChannel(context.Context, int64, int64) error              { return nil }
func (f *voiceFakeDB) SetChannel(context.Context, int64, int64, []byte) error         { return nil }
func (f *voiceFakeDB) SetVoiceState(context.Context, int64, int64, []byte) error      { return nil }
func (f *voiceFakeDB) GetGuildVoiceSnapshot(context.Context, int64, int64, time.Duration) (*state.VoiceSnapshot, error) {
	return nil, nil
}

const trackedGuild = int64(1347103391402164315)

func newVoiceClient(t *testing.T, db *voiceFakeDB) *Client {
	c := NewClient(slogtest.Make(t, nil), db)
	c.SetVoiceGuilds(map[int64]struct{}{trackedGuild: {}})
	return c
}

func receipt() discord.Receipt {
	return discord.Receipt{ShardID: 3, ConnectionID: "sess-1", ReceivedAt: time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)}
}

func TestVoiceStateUpdateStoresOnlyTrackedGuilds(t *testing.T) {
	db := &voiceFakeDB{}
	c := newVoiceClient(t, db)

	other := &discord.Event{T: "VOICE_STATE_UPDATE", S: 9, Receipt: receipt(),
		D: []byte(`{"guild_id":"42","user_id":"7","channel_id":"5","member":{"user":{"id":"7"}}}`)}
	if _, err := c.HandleEvent(context.Background(), other); err != nil || len(db.applied) != 0 {
		t.Fatalf("other guilds' voice traffic must stay unstored: err=%v applied=%v", err, db.applied)
	}

	tracked := &discord.Event{T: "VOICE_STATE_UPDATE", S: 9, Receipt: receipt(),
		D: []byte(`{"guild_id":"1347103391402164315","user_id":"7","channel_id":"5","member":{"user":{"id":"7","bot":true}}}`)}
	if _, err := c.HandleEvent(context.Background(), tracked); err != nil {
		t.Fatalf("handle: %v", err)
	}
	got := db.applied[0]
	if got.GuildID != trackedGuild || got.UserID != 7 || got.ChannelID != 5 || got.IsBot == nil || !*got.IsBot ||
		got.DiscordSeq != 9 || !got.ChangedAt.Equal(receipt().ReceivedAt) {
		t.Fatalf("stored change must carry location, classification and receipt: %+v", got)
	}

	db.fail = errors.New("db down")
	_, err := c.HandleEvent(context.Background(), tracked)
	var voiceErr *VoiceStateError
	if !errors.As(err, &voiceErr) || voiceErr.GuildID != trackedGuild {
		t.Fatalf("a failed write must surface as a VoiceStateError, got %v", err)
	}
}

func TestGuildCreateBaselineOnlyFromCompleteGuildCreate(t *testing.T) {
	db := &voiceFakeDB{}
	c := newVoiceClient(t, db)

	create := &discord.Event{T: "GUILD_CREATE", S: 4, Receipt: receipt(), D: []byte(`{
		"id":"1347103391402164315","member_count":3,
		"members":[{"user":{"id":"10"}},{"user":{"id":"11","bot":true}}],
		"voice_states":[
			{"user_id":"10","channel_id":"5"},
			{"user_id":"11","channel_id":"5"},
			{"user_id":"12","channel_id":"5"}
		]}`)}
	payload, err := c.HandleEvent(context.Background(), create)
	if err != nil || payload == nil {
		t.Fatalf("handle: %v", err)
	}
	if len(db.baselines) != 1 {
		t.Fatalf("GUILD_CREATE must replace the voice baseline")
	}
	b := db.baselines[0]
	if b.ShardID != 3 || b.ConnectionID != "sess-1" || b.DiscordSeq != 4 || len(b.Occupants) != 3 {
		t.Fatalf("baseline identity: %+v", b)
	}
	classes := map[int64]*bool{}
	for _, o := range b.Occupants {
		classes[o.UserID] = o.IsBot
	}
	if classes[10] == nil || *classes[10] || classes[11] == nil || !*classes[11] || classes[12] != nil {
		t.Fatalf("members classify occupants; an unlisted one stays unknown: %v %v %v", classes[10], classes[11], classes[12])
	}

	update := &discord.Event{T: "GUILD_UPDATE", Receipt: receipt(), D: []byte(`{"id":"1347103391402164315","member_count":3}`)}
	if _, err := c.HandleEvent(context.Background(), update); err != nil || len(db.baselines) != 1 || len(db.cleared) != 0 {
		t.Fatalf("GUILD_UPDATE carries no voice states and must not touch the baseline: err=%v", err)
	}

	unavailable := &discord.Event{T: "GUILD_CREATE", Receipt: receipt(), D: []byte(`{"id":"1347103391402164315","unavailable":true,"member_count":1}`)}
	if _, err := c.HandleEvent(context.Background(), unavailable); err != nil || len(db.cleared) != 1 || len(db.baselines) != 1 {
		t.Fatalf("an unavailable guild must clear readiness, never store empty presence: err=%v", err)
	}

	untracked := &discord.Event{T: "GUILD_CREATE", Receipt: receipt(), D: []byte(`{"id":"42","member_count":1,"voice_states":[]}`)}
	if _, err := c.HandleEvent(context.Background(), untracked); err != nil || len(db.baselines) != 1 {
		t.Fatalf("untracked guilds get no baseline: err=%v", err)
	}
}

func TestGuildCreateVoiceFailureKeepsPayload(t *testing.T) {
	db := &voiceFakeDB{fail: errors.New("db down")}
	c := newVoiceClient(t, db)
	create := &discord.Event{T: "GUILD_CREATE", Receipt: receipt(), D: []byte(`{"id":"1347103391402164315","member_count":1,"voice_states":[]}`)}
	payload, err := c.HandleEvent(context.Background(), create)
	var voiceErr *VoiceStateError
	if !errors.As(err, &voiceErr) || payload == nil || payload.GuildID != trackedGuild {
		t.Fatalf("a voice failure must not drop the guild payload: payload=%v err=%v", payload, err)
	}
}

func TestDeletionsClearTrackedPresence(t *testing.T) {
	db := &voiceFakeDB{}
	c := newVoiceClient(t, db)
	if _, err := c.HandleEvent(context.Background(), &discord.Event{T: "CHANNEL_DELETE", D: []byte(`{"id":"5","guild_id":"1347103391402164315"}`)}); err != nil {
		t.Fatalf("channel delete: %v", err)
	}
	if len(db.channels) != 1 || db.channels[0] != [2]int64{trackedGuild, 5} {
		t.Fatalf("deleted channel must drop its occupants: %v", db.channels)
	}
	if _, err := c.HandleEvent(context.Background(), &discord.Event{T: "GUILD_DELETE", D: []byte(`{"id":"1347103391402164315","unavailable":true}`)}); err != nil {
		t.Fatalf("guild delete: %v", err)
	}
	if len(db.cleared) != 1 {
		t.Fatalf("guild delete must clear presence and readiness")
	}
}
