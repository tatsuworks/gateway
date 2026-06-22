package discordjson

import (
	"testing"

	"cdr.dev/slog/sloggers/slogtest/assert"
)

func TestDecodeMemberChunkFields(t *testing.T) {
	buf := []byte(`{"guild_id":"7","members":[],"chunk_index":1,"chunk_count":3}`)

	mc, err := (decoder{}).DecodeMemberChunk(buf)
	assert.Success(t, "decode member chunk", err)
	assert.Equal(t, "guild id", int64(7), mc.GuildID)
	assert.Equal(t, "chunk index", 1, mc.ChunkIndex)
	assert.Equal(t, "chunk count", 3, mc.ChunkCount)
}
