package state

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/gateway/discordetf"
)

func (s *Client) ChannelCreate(d []byte) error {
	ch, err := discordetf.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtChannelKey(ch.Guild, ch.Id), ch.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) ChannelDelete(d []byte) error {
	ch, err := discordetf.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.Transact(func(t fdb.Transaction) error {
		t.Clear(c.fmtChannelKey(ch.Guild, ch.Id))
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) VoiceStateUpdate(d []byte) error {
	vs, err := discordetf.DecodeVoiceState(d)
	if err != nil {
		return err
	}

	err = c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtVoiceStateKey(vs.Channel, vs.User), vs.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
