package handler

import (
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func (c *Client) GuildCreate(d []byte) (guild int64, _ error) {
	gc, err := c.enc.DecodeGuildCreate(d)
	if err != nil {
		return 0, xerrors.Errorf("parse guild create: %w", err)
	}

	if gc.MemberCount > int64(len(gc.Members)) {
		guild = gc.ID
	}

	eg := new(errgroup.Group)
	eg.Go(func() error {
		err := c.db.SetGuild(defaultCtx, gc.ID, gc.Raw)
		if err != nil {
			return xerrors.Errorf("set guild: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			err := c.db.SetGuildRoles(defaultCtx, gc.ID, gc.Roles)
			if err != nil {
				return xerrors.Errorf("set guild roles: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			err := c.db.SetGuildMembers(defaultCtx, gc.ID, gc.Members)
			if err != nil {
				return xerrors.Errorf("set guild member: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			err := c.db.SetChannels(defaultCtx, gc.ID, gc.Channels)
			if err != nil {
				return xerrors.Errorf("set guild channels: %w", err)
			}

		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Emojis) > 0 {
			err := c.db.SetGuildEmojis(defaultCtx, gc.ID, gc.Emojis)
			if err != nil {
				return xerrors.Errorf("set guild emojis: %w", err)
			}
		}
		return nil
	})

	return guild, eg.Wait()
}

func (c *Client) GuildDelete(d []byte) error {
	gc, err := c.enc.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		err := c.db.DeleteGuild(defaultCtx, gc.ID)
		if err != nil {
			return xerrors.Errorf("delete guild: %w", err)
		}

		return nil
	})

	// eg.Go(func() error {
	// 	err := c.db.DeleteGuildRoles(defaultCtx, gc.ID)
	// 	if err != nil {
	// 		return xerrors.Errorf("delete guild roles: %w", err)
	// 	}
	//
	// 	return nil
	// })
	// eg.Go(func() error {
	// 	err := c.db.DeleteGuildMembers(defaultCtx, gc.ID)
	// 	if err != nil {
	// 		return xerrors.Errorf("delete guild members: %w", err)
	// 	}
	//
	// 	return nil
	// })
	// eg.Go(func() error {
	// 	err := c.db.DeleteChannels(defaultCtx, gc.ID)
	// 	if err != nil {
	// 		return xerrors.Errorf("delete channels: %w", err)
	// 	}
	//
	// 	return nil
	// })

	return eg.Wait()
}

func (c *Client) GuildBanAdd(d []byte) error {
	gb, err := c.enc.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildBan(defaultCtx, gb.GuildID, gb.UserID, gb.Raw)
	if err != nil {
		return xerrors.Errorf("set guild ban: %w", err)
	}
	return nil
}

func (c *Client) GuildBanRemove(d []byte) error {
	gb, err := c.enc.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildBan(defaultCtx, gb.GuildID, gb.UserID)
	if err != nil {
		return xerrors.Errorf("delete guild ban: %w", err)
	}
	return nil
}
