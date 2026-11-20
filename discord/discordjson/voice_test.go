package discordjson

import "testing"

func TestDecodeVoiceStateClassifiesAccounts(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		channel int64
		isBot   *bool
	}{
		{"human joins", `{"guild_id":"1","user_id":"2","channel_id":"3","member":{"user":{"id":"2"}}}`, 3, boolp(false)},
		{"bot joins", `{"guild_id":"1","user_id":"2","channel_id":"3","member":{"user":{"id":"2","bot":true}}}`, 3, boolp(true)},
		{"no member is unknown", `{"guild_id":"1","user_id":"2","channel_id":"3"}`, 3, nil},
		{"departure", `{"guild_id":"1","user_id":"2","channel_id":null,"member":{"user":{"id":"2"}}}`, 0, boolp(false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs, err := Encoding.DecodeVoiceState([]byte(tc.raw))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if vs.GuildID != 1 || vs.UserID != 2 || vs.ChannelID != tc.channel {
				t.Fatalf("ids: %+v", vs)
			}
			if (vs.IsBot == nil) != (tc.isBot == nil) || (vs.IsBot != nil && *vs.IsBot != *tc.isBot) {
				t.Fatalf("isBot = %v, want %v", deref(vs.IsBot), deref(tc.isBot))
			}
		})
	}
}

func TestDecodeGuildCreateVoiceStatePresence(t *testing.T) {
	gc, err := Encoding.DecodeGuildCreate([]byte(`{"id":"1","member_count":2,"voice_states":[]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gc.VoiceStatesPresent || gc.Unavailable || len(gc.VoiceStates) != 0 {
		t.Fatalf("an empty voice_states list is a verified empty baseline: %+v", gc)
	}

	gc, err = Encoding.DecodeGuildCreate([]byte(`{"id":"1","member_count":2,"name":"x"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gc.VoiceStatesPresent {
		t.Fatalf("a payload without voice_states must not read as an empty baseline")
	}

	gc, err = Encoding.DecodeGuildCreate([]byte(`{"id":"1","unavailable":true}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gc.Unavailable || string(gc.Raw) != `{"id":"1","unavailable":true}` {
		t.Fatalf("unavailable must be read and kept in the stored guild: %+v %s", gc, gc.Raw)
	}
}

func boolp(b bool) *bool { return &b }

func deref(b *bool) interface{} {
	if b == nil {
		return nil
	}
	return *b
}
