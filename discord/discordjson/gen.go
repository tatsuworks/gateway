package discordjson

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/tatsuworks/gateway/discord"
	"golang.org/x/xerrors"
)

type event struct {
	D  RawMessage `json:"d"`
	Op int        `json:"op"`
	S  int64      `json:"s"`
	T  string     `json:"t"`
}

func (_ decoder) DecodeT(buf []byte) (*discord.Event, error) {
	var (
		e = event{}
	)

	err := jsoniter.Unmarshal(buf, &e)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal event: %w", err)
	}

	// Copy rather than cast: discord.Event is larger (Receipt), and a cast
	// would let the gateway's receipt stamp write past this allocation.
	return &discord.Event{D: e.D, Op: e.Op, S: e.S, T: e.T}, nil
}
