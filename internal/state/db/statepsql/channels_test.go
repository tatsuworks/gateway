package statepsql

import (
	"context"
	"math/rand"
	"testing"

	"cdr.dev/slog/sloggers/slogtest/assert"
)

var channelJSON = []byte(`{"id": "41771983423143937", "name": "general", "nsfw": true, "type": 0, "topic": "24/7 chat about how to gank Mike #2", "guild_id": "41771983423143937", "position": 6, "parent_id": "399942396007890945", "last_message_id": "155117677105512449", "rate_limit_per_user": 2, "permission_overwrites": []}`)

func TestChannels(t *testing.T) {
	db, err := NewDB(context.Background(), "postgresql://tatsu@localhost/tatsu?sslmode=disable")
	assert.Success(t, "failed to open postgres", err)

	var (
		ctx   = context.Background()
		id    = rand.Int63()
		guild = rand.Int63()
	)

	err = db.SetChannel(ctx, guild, id, channelJSON)
	assert.Success(t, "failed to set channel", err)

	data, err := db.GetChannel(ctx, id)
	assert.Success(t, "failed to get channel", err)

	assert.Equal(t, "expected channels to be equal", channelJSON, data)

	err = db.DeleteChannel(ctx, guild, id)
	assert.Success(t, "failed to delete channel", err)

	guild = rand.Int63()
	expected := map[int64][]byte{}
	for i := 0; i < 5; i++ {
		expected[rand.Int63()] = channelJSON
	}

	err = db.SetChannels(ctx, guild, expected)
	assert.Success(t, "failed to bulk set channels", err)
}
