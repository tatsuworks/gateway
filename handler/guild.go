package handler

import (
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discordetf"
)

func (c *Client) GuildCreate(d []byte) (guild int64, _ error) {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return 0, xerrors.Errorf("parse guild create: %w", err)
	}

	if gc.MemberCount > int64(len(gc.Members)) {
		guild = gc.Id
	}

	eg := new(errgroup.Group)
	eg.Go(func() error {
		err := c.db.SetGuild(gc.Id, gc.Guild)
		if err != nil {
			return xerrors.Errorf("set guild: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			err := c.db.SetGuildRoles(gc.Id, gc.Roles)
			if err != nil {
				return xerrors.Errorf("set guild roles: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			err := c.db.SetGuildMembers(gc.Id, gc.Members)
			if err != nil {
				return xerrors.Errorf("set guild member: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			err := c.db.SetChannels(gc.Id, gc.Channels)
			if err != nil {
				return xerrors.Errorf("set guild channels: %w", err)
			}

		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Emojis) > 0 {
			err := c.db.SetGuildEmojis(gc.Id, gc.Emojis)
			if err != nil {
				return xerrors.Errorf("set guild emojis: %w", err)
			}
		}
		return nil
	})

	return guild, eg.Wait()
}

func (c *Client) GuildDelete(d []byte) error {
	gc, err := discordetf.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		err := c.db.DeleteGuild(gc.Id)
		if err != nil {
			return xerrors.Errorf("delete guild: %w", err)
		}

		return nil
	})

	eg.Go(func() error {
		err := c.db.DeleteGuildRoles(gc.Id)
		if err != nil {
			return xerrors.Errorf("delete guild roles: %w", err)
		}

		return nil
	})
	eg.Go(func() error {
		err := c.db.DeleteGuildMembers(gc.Id)
		if err != nil {
			return xerrors.Errorf("delete guild members: %w", err)
		}

		return nil
	})
	eg.Go(func() error {
		err := c.db.DeleteChannels(gc.Id)
		if err != nil {
			return xerrors.Errorf("delete channels: %w", err)
		}

		return nil
	})

	return eg.Wait()
}

func (c *Client) GuildBanAdd(d []byte) error {
	gb, err := discordetf.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildBan(gb.Guild, gb.User, gb.Raw)
	if err != nil {
		return xerrors.Errorf("set guild ban: %w", err)
	}
	return nil
}

func (c *Client) GuildBanRemove(d []byte) error {
	gb, err := discordetf.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildBan(gb.Guild, gb.User)
	if err != nil {
		return xerrors.Errorf("delete guild ban: %w", err)
	}
	return nil
}
