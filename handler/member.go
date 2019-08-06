package handler

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/tatsuworks/gateway/discordetf"
)

func (c *Client) MemberChunk(d []byte) error {
	mc, err := discordetf.DecodeMemberChunk(d)
	if err != nil {
		return err
	}

	return c.setGuildETFs(mc.Guild, mc.Members, c.fmtGuildMemberKey)
}

func (c *Client) MemberAdd(d []byte) error {
	mc, err := discordetf.DecodeMember(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtGuildMemberKey(mc.Guild, mc.Id), mc.Raw)
		return nil
	})
}

func (c *Client) MemberRemove(d []byte) error {
	mc, err := discordetf.DecodeMember(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Clear(c.fmtGuildMemberKey(mc.Guild, mc.Id))
		return nil
	})
}

func (c *Client) PresenceUpdate(d []byte) error {
	if true {
		return nil
	}

	p, err := discordetf.DecodePresence(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtGuildPresenceKey(p.Guild, p.Id), p.Raw)
		return nil
	})
}
