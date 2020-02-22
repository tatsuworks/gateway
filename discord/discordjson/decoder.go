package discordjson

import (
	"encoding/json"

	"github.com/tatsuworks/gateway/discord"
	"golang.org/x/xerrors"
)

var Encoding discord.Encoding = &decoder{}

type decoder struct{}

func (_ decoder) Name() string {
	return "json"
}

var _ json.RawMessage

type RawMessage []byte

// MarshalJSON returns m as the JSON encoding of m.
func (m RawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalJSON sets *m to a copy of data.
func (m *RawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return xerrors.New("json.RawMessage: UnmarshalJSON on nil pointer")
	}
	*m = data
	return nil
}
