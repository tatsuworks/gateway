package handler

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/tatsuworks/gateway/discordetf"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func (c *Client) GuildCreate(d []byte) error {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			t.Set(c.fmtGuildKey(gc.Id), gc.Guild)
			return nil
		})
	})

	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			return c.setGuildETFs(gc.Id, gc.Roles, c.fmtGuildRoleKey)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			return c.setGuildETFs(gc.Id, gc.Members, c.fmtGuildMemberKey)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			err := c.setGuildETFs(gc.Id, gc.Channels, c.fmtGuildChannelKey)
			if err != nil {
				return xerrors.Errorf("failed to set guild channel keys: %w", err)
			}

			err = c.setETFs(gc.Id, gc.Channels, c.fmtChannelKey)
			if err != nil {
				return xerrors.Errorf("failed to set channel keys: %w", err)
			}

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
			t.Clear(c.fmtGuildKey(gc.Id))
			return nil
		})
	})

	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(c.fmtGuildRolePrefix(gc.Id))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})
	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(c.fmtGuildMemberPrefix(gc.Id))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})
	eg.Go(func() error {
		return c.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(c.fmtGuildChannelPrefix(gc.Id))
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