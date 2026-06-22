package discordetf

import (
	"encoding/binary"
	"testing"

	"cdr.dev/slog/sloggers/slogtest/assert"
)

// atom encodes an ETF atom term: tag + 2-byte big-endian length + bytes.
func atom(s string) []byte {
	b := []byte{ettAtom, 0, 0}
	binary.BigEndian.PutUint16(b[1:3], uint16(len(s)))
	return append(b, []byte(s)...)
}

// smallInt encodes an ETF small-integer term: tag + 1 byte.
func smallInt(n byte) []byte { return []byte{ettSmallInteger, n} }

// smallBig encodes a non-negative ETF small-big term: tag + len + sign(0) + 1 magnitude byte.
func smallBig(n byte) []byte { return []byte{ettSmallBig, 0x01, 0x00, n} }

// binaryTerm encodes an ETF binary term: tag + 4-byte big-endian length + bytes.
func binaryTerm(s string) []byte {
	b := []byte{ettBinary, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:5], uint32(len(s)))
	return append(b, []byte(s)...)
}

func TestDecodeMemberChunkFields(t *testing.T) {
	var buf []byte
	// map header: ettMap + 4-byte big-endian arity (5 pairs)
	hdr := []byte{ettMap, 0, 0, 0, 5}
	buf = append(buf, hdr...)
	buf = append(buf, atom("guild_id")...)
	buf = append(buf, smallBig(7)...)
	buf = append(buf, atom("members")...)
	buf = append(buf, ettNil) // empty members list
	buf = append(buf, atom("nonce")...)
	buf = append(buf, binaryTerm("abc")...) // unknown key, must be skipped
	buf = append(buf, atom("chunk_index")...)
	buf = append(buf, smallInt(1)...)
	buf = append(buf, atom("chunk_count")...)
	buf = append(buf, smallInt(3)...)

	mc, err := (decoder{}).DecodeMemberChunk(buf)
	assert.Success(t, "decode member chunk", err)
	assert.Equal(t, "guild id", int64(7), mc.GuildID)
	assert.Equal(t, "chunk index", 1, mc.ChunkIndex)
	assert.Equal(t, "chunk count", 3, mc.ChunkCount)
}
