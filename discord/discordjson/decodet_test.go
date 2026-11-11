package discordjson

import (
	"testing"
	"time"

	"github.com/tatsuworks/gateway/discord"
)

// The gateway stamps Receipt on every decoded dispatch, so DecodeT must return
// a whole discord.Event. Run under -race (checkptr) to catch a cast from a
// smaller struct, which writes Receipt past the allocation.
func TestDecodeTReturnsWholeEvent(t *testing.T) {
	ev, err := Encoding.DecodeT([]byte(`{"op":0,"s":7,"t":"GUILD_CREATE","d":{"id":"1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	ev.Receipt = discord.Receipt{ShardID: 2, ConnectionID: "sess", ReceivedAt: time.Unix(1, 0)}
	if ev.Op != 0 || ev.S != 7 || ev.T != "GUILD_CREATE" || string(ev.D) != `{"id":"1"}` || ev.Receipt.ConnectionID != "sess" {
		t.Fatalf("decoded event: %+v", ev)
	}
}
