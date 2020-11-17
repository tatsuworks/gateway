package handler

import (
	"context"
	"encoding/json"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func (c *Client) GuildCreate(ctx context.Context, d []byte) (*EventPayload, error) {
	gc, err := c.enc.DecodeGuildCreate(d)
	if err != nil {
		return nil, xerrors.Errorf("parse guild create: %w", err)
	}

	guild := gc.ID
	result := &EventPayload{
		GuildID: guild,
	}
	// if true {
	// 	return guild, nil
	// }

	eg := new(errgroup.Group)
	eg.Go(func() error {
		if gc.MemberCount == 0 {
			mc, err := c.db.GetGuildMemberCount(ctx, guild)
			if err != nil {
				return xerrors.Errorf("GetGuildMemberCount: %w", err)
			}
			var data map[string]interface{}
			err = json.Unmarshal(gc.Raw, &data)
			if err != nil {
				return xerrors.Errorf("GetGuildMemberCount json Unmarshal: %w", err)
			}
			data["member_count"] = mc
			gc.Raw, err = json.Marshal(data)
			if err != nil {
				return xerrors.Errorf("GetGuildMemberCount json Marshal: %w", err)
			}
		}
		isNewGuild, err := c.db.SetGuild(ctx, gc.ID, gc.Raw)
		if err != nil {
			return xerrors.Errorf("set guild: %w", err)
		}
		result.IsNewGuild = isNewGuild
		return nil
	})
	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			err := c.db.SetGuildRoles(ctx, gc.ID, gc.Roles)
			if err != nil {
				return xerrors.Errorf("set guild roles: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			err := c.db.SetGuildMembers(ctx, gc.ID, gc.Members)
			if err != nil {
				return xerrors.Errorf("set guild member: %w", err)
			}
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			err := c.db.SetChannels(ctx, gc.ID, gc.Channels)
			if err != nil {
				return xerrors.Errorf("set guild channels: %w", err)
			}

		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Emojis) > 0 {
			err := c.db.SetGuildEmojis(ctx, gc.ID, gc.Emojis)
			if err != nil {
				return xerrors.Errorf("set guild emojis: %w", err)
			}
		}
		return nil
	})
	err = eg.Wait()
	return result, err
}

func (c *Client) GuildDelete(ctx context.Context, d []byte) error {
	gc, err := c.enc.DecodeGuildCreate(d)
	if err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		err := c.db.DeleteGuild(ctx, gc.ID)
		if err != nil {
			return xerrors.Errorf("delete guild: %w", err)
		}

		return nil
	})

	// eg.Go(func() error {
	// 	err := c.db.DeleteGuildRoles(ctx, gc.ID)
	// 	if err != nil {
	// 		return xerrors.Errorf("delete guild roles: %w", err)
	// 	}
	//
	// 	return nil
	// })
	// eg.Go(func() error {
	// 	err := c.db.DeleteGuildMembers(ctx, gc.ID)
	// 	if err != nil {
	// 		return xerrors.Errorf("delete guild members: %w", err)
	// 	}
	//
	// 	return nil
	// })
	// eg.Go(func() error {
	// 	err := c.db.DeleteChannels(ctx, gc.ID)
	// 	if err != nil {
	// 		return xerrors.Errorf("delete channels: %w", err)
	// 	}
	//
	// 	return nil
	// })

	return eg.Wait()
}

func (c *Client) GuildBanAdd(ctx context.Context, d []byte) error {
	gb, err := c.enc.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.SetGuildBan(ctx, gb.GuildID, gb.UserID, gb.Raw)
	if err != nil {
		return xerrors.Errorf("set guild ban: %w", err)
	}
	return nil
}

func (c *Client) GuildBanRemove(ctx context.Context, d []byte) error {
	gb, err := c.enc.DecodeGuildBan(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteGuildBan(ctx, gb.GuildID, gb.UserID)
	if err != nil {
		return xerrors.Errorf("delete guild ban: %w", err)
	}
	return nil
}
