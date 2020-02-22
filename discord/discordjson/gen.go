package discordjson

import (
	"unsafe"

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

	// fmt.Printf("t: %s, s: %d, op: %d\n", e.T, e.S, e.Op)
	return (*discord.Event)(unsafe.Pointer(&e)), nil
}
