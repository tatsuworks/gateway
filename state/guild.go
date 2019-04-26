package state

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/gateway/discordetf"
	"golang.org/x/sync/errgroup"
)

func (c *Client) GuildCreate(d []byte) error {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			return c.setETFs(gc.Id, gc.Roles, c.fmtRoleKey)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			return c.setETFs(gc.Id, gc.Members, c.fmtMemberKey)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			return c.setETFs(gc.Id, gc.Channels, c.fmtChannelKey)
		}
		return nil
	})

	return eg.Wait()
}

func (c *Client) GuildDelete(d []byte) error {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(c.fmtRoleKey(gc.Id, 0))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})
	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(c.fmtMemberKey(gc.Id, 0))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})
	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(c.fmtChannelKey(gc.Id, 0))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})

	return eg.Wait()
}

func (c *Client) GuildBanAdd(d []byte) error {
	gb, err := discordetf.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtGuildBanKey(gb.Guild, gb.User), gb.Raw)
		return nil
	})
}

func (c *Client) GuildBanRemove(d []byte) error {
	gb, err := discordetf.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Clear(c.fmtGuildBanKey(gb.Guild, gb.User))
		return nil
	})
}
