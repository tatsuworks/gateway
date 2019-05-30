package state

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/tatsuworks/gateway/discordetf"
	"github.com/pkg/errors"
)

func (c *Client) MessageCreate(d []byte) error {
	mc, err := discordetf.DecodeMessage(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtMessageKey(mc.Channel, mc.Id), mc.Raw)
		return nil
	})
}

func (c *Client) MessageDelete(d []byte) error {
	mc, err := discordetf.DecodeMessage(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Clear(c.fmtMessageKey(mc.Channel, mc.Id))
		return nil
	})
}

func (c *Client) MessageReactionAdd(d []byte) error {
	rc, err := discordetf.DecodeMessageReaction(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtMessageReactionKey(rc.Channel, rc.Message, rc.User, rc.Name), rc.Raw)
		return nil
	})
}

func (c *Client) MessageReactionRemove(d []byte) error {
	rc, err := discordetf.DecodeMessageReaction(d)
	if err != nil {
		return err
	}
	return c.Transact(func(t fdb.Transaction) error {
		t.Clear(c.fmtMessageReactionKey(rc.Channel, rc.Message, rc.User, rc.Name))
		return nil
	})
}

func (c *Client) MessageReactionRemoveAll(d []byte) error {
	rc, err := discordetf.DecodeMessageReactionRemoveAll(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		pre, err := fdb.PrefixRange(c.fmtMessageReactionKey(rc.Channel, rc.Message, rc.User, ""))
		if err != nil {
			return errors.Wrap(err, "failed to make message reaction prefixrange")
		}

		t.ClearRange(pre)
		return nil
	})
}
