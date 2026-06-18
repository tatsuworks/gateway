package gatewayws

import (
	"testing"

	"github.com/tatsuworks/gateway/handler"
)

func TestMembersComplete(t *testing.T) {
	cases := []struct {
		name string
		p    handler.EventPayload
		want bool
	}{
		{"small full", handler.EventPayload{MemberCount: 10, ReceivedMembers: 10}, true},
		{"small over-received", handler.EventPayload{MemberCount: 10, ReceivedMembers: 12}, true},
		{"small partial", handler.EventPayload{MemberCount: 10, ReceivedMembers: 4}, false},
		{"at threshold", handler.EventPayload{MemberCount: LargeThreshold, ReceivedMembers: LargeThreshold}, true},
		{"large guild", handler.EventPayload{MemberCount: LargeThreshold + 1, ReceivedMembers: LargeThreshold + 1}, false},
		{"zero count", handler.EventPayload{MemberCount: 0, ReceivedMembers: 0}, false},
	}
	for _, c := range cases {
		if got := membersComplete(&c.p, LargeThreshold); got != c.want {
			t.Errorf("%s: membersComplete = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestJitterFactorBounds(t *testing.T) {
	for _, id := range []int64{0, 1, 7, 173184118492889089, -5, 1 << 40} {
		f := jitterFactor(id)
		if f < 0.75 || f >= 1.25 {
			t.Errorf("jitterFactor(%d) = %v, want [0.75, 1.25)", id, f)
		}
		if jitterFactor(id) != f {
			t.Errorf("jitterFactor(%d) not deterministic", id)
		}
	}
}
