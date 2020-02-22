package discordjson

import (
	"strconv"
	"testing"

	jsoniter "github.com/json-iterator/go"
)

var channelJSON = []byte(`{"id": "41771983423143937", "name": "general", "nsfw": true, "type": 0, "topic": "24/7 chat about how to gank Mike #2", "guild_id": "41771983423143937", "position": 6, "parent_id": "399942396007890945", "last_message_id": "155117677105512449", "rate_limit_per_user": 2, "permission_overwrites": []}`)

func getInt() (*channelInt, error) {
	idStr := jsoniter.Get(channelJSON, "id").ToString()
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, err
	}

	guildIDStr := jsoniter.Get(channelJSON, "guild_id").ToString()
	guildID, err := strconv.ParseInt(guildIDStr, 10, 64)
	if err != nil {
		return nil, err
	}

	return &channelInt{
		ID:      id,
		GuildID: guildID,
		Raw:     channelJSON,
	}, nil
}

func getUint() (*channelUint, error) {
	idStr := jsoniter.Get(channelJSON, "id").ToString()
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return nil, err
	}

	guildIDStr := jsoniter.Get(channelJSON, "guild_id").ToString()
	guildID, err := strconv.ParseUint(guildIDStr, 10, 64)
	if err != nil {
		return nil, err
	}

	return &channelUint{
		ID:      id,
		GuildID: guildID,
		Raw:     channelJSON,
	}, nil
}

func getUintToInt() (*channelInt, error) {
	idStr := jsoniter.Get(channelJSON, "id").ToString()
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return nil, err
	}

	guildIDStr := jsoniter.Get(channelJSON, "guild_id").ToString()
	guildID, err := strconv.ParseUint(guildIDStr, 10, 64)
	if err != nil {
		return nil, err
	}

	return &channelInt{
		ID:      int64(id),
		GuildID: int64(guildID),
		Raw:     channelJSON,
	}, nil
}

type channelInt struct {
	ID      int64
	GuildID int64
	Raw     []byte
}

type channelUint struct {
	ID      uint64
	GuildID uint64
	Raw     []byte
}

var outInt *channelInt
var outUint *channelUint

func BenchmarkDecodeChannel_int(b *testing.B) {
	var (
		c   *channelInt
		err error
	)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, err = getInt()
		if err != nil {
			b.Fatalf("failed to decode: %s", err.Error())
		}
	}

	outInt = c
}

func BenchmarkDecodeChannel_uint(b *testing.B) {
	var (
		c   *channelUint
		err error
	)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, err = getUint()
		if err != nil {
			b.Fatalf("failed to decode: %s", err.Error())
		}
	}

	outUint = c
}

func BenchmarkDecodeChannel_uintToInt(b *testing.B) {
	var (
		c   *channelInt
		err error
	)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c, err = getUintToInt()
		if err != nil {
			b.Fatalf("failed to decode: %s", err.Error())
		}
	}

	outInt = c
}
