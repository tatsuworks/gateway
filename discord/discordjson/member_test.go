package discordjson

import (
	"testing"
)

func TestDecodeMemberChunk_PaginationFields(t *testing.T) {
	payload := []byte(`{
		"guild_id":"41771983444115456",
		"members":[{"user":{"id":"80351110224678912"}}],
		"chunk_index":3,
		"chunk_count":10
	}`)

	mc, err := (decoder{}).DecodeMemberChunk(payload)
	if err != nil {
		t.Fatalf("decode err: %v", err)
	}
	if mc.GuildID != 41771983444115456 {
		t.Fatalf("guild_id mismatch: %d", mc.GuildID)
	}
	if mc.ChunkIndex != 3 {
		t.Fatalf("chunk_index: got %d want 3", mc.ChunkIndex)
	}
	if mc.ChunkCount != 10 {
		t.Fatalf("chunk_count: got %d want 10", mc.ChunkCount)
	}
}

func TestDecodeMemberChunk_PaginationDefaults(t *testing.T) {
	payload := []byte(`{
		"guild_id":"41771983444115456",
		"members":[]
	}`)

	mc, err := (decoder{}).DecodeMemberChunk(payload)
	if err != nil {
		t.Fatalf("decode err: %v", err)
	}
	if mc.ChunkIndex != 0 || mc.ChunkCount != 0 {
		t.Fatalf("expected zero defaults, got %d/%d", mc.ChunkIndex, mc.ChunkCount)
	}
}
