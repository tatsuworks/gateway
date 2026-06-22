package gatewaypb

import (
	"testing"

	"github.com/gogo/protobuf/proto"
)

func TestRestartShardRequestForceIdentifyRoundTrip(t *testing.T) {
	in := &RestartShardRequest{
		Shard:         42,
		ForceIdentify: true,
	}

	raw, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out RestartShardRequest
	if err := proto.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.GetShard() != 42 {
		t.Fatalf("shard = %d, want 42", out.GetShard())
	}
	if !out.GetForceIdentify() {
		t.Fatal("force_identify = false, want true")
	}
}
