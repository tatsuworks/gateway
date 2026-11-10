package handler

import (
	"context"
	"encoding/json"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/discord/discordjson"
	"github.com/tatsuworks/gateway/internal/state"
)

func (c *Client) GuildCreate(ctx context.Context, d []byte) (*EventPayload, error) {
	return c.guildCreate(ctx, &discord.Event{T: "GUILD_UPDATE", D: d}, false)
}

// guildCreate stores a GUILD_CREATE or GUILD_UPDATE. Only a GUILD_CREATE is a
// voice baseline: GUILD_UPDATE never carries voice states.
func (c *Client) guildCreate(ctx context.Context, e *discord.Event, baseline bool) (*EventPayload, error) {
	gc, err := c.enc.DecodeGuildCreate(e.D)
	if err != nil {
		return nil, xerrors.Errorf("parse guild create: %w", err)
	}

	eg := new(errgroup.Group)
	eg.Go(func() error {
		if gc.MemberCount == 0 {
			mc, err := c.db.GetGuildMemberCount(ctx, gc.ID)
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
		err := c.db.SetGuild(ctx, gc.ID, gc.Raw)
		if err != nil {
			return xerrors.Errorf("set guild: %w", err)
		}
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
	eg.Go(func() error {
		if len(gc.Threads) > 0 {
			err := c.db.SetThreads(ctx, gc.ID, gc.Threads)
			if err != nil {
				return xerrors.Errorf("set guild threads: %w", err)
			}

		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Presences) > 0 {
			err := c.db.SetPresences(ctx, gc.ID, gc.Presences)
			if err != nil {
				return xerrors.Errorf("set guild presences: %w", err)
			}

		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	memberIDs := make([]int64, 0, len(gc.Members))
	for id := range gc.Members {
		memberIDs = append(memberIDs, id)
	}

	payload := &EventPayload{
		GuildID:         gc.ID,
		MemberCount:     gc.MemberCount,
		ReceivedMembers: len(gc.Members),
		MemberIDs:       memberIDs,
	}

	if baseline && c.TracksVoice(gc.ID) {
		if err := c.replaceVoiceBaseline(ctx, e, gc); err != nil {
			return payload, &VoiceStateError{GuildID: gc.ID, Err: err}
		}
	}

	return payload, nil
}

// replaceVoiceBaseline makes a complete GUILD_CREATE the guild's voice
// presence. An unavailable guild carries no state at all, so it clears the
// presence and its readiness instead of storing an empty channel list as if
// it were real; so does a payload that somehow lacks voice_states.
func (c *Client) replaceVoiceBaseline(ctx context.Context, e *discord.Event, gc *discord.GuildCreate) error {
	if gc.Unavailable || !gc.VoiceStatesPresent || e.Receipt.ConnectionID == "" {
		if err := c.db.ClearGuildVoiceStates(ctx, gc.ID); err != nil {
			return xerrors.Errorf("clear voice baseline: %w", err)
		}
		return nil
	}

	baseline := state.VoiceBaseline{
		GuildID:      gc.ID,
		ShardID:      e.Receipt.ShardID,
		ConnectionID: e.Receipt.ConnectionID,
		ReceivedAt:   e.Receipt.ReceivedAt,
		DiscordSeq:   e.S,
		Occupants:    make([]state.VoiceOccupant, 0, len(gc.VoiceStates)),
		Raws:         gc.VoiceStates,
	}
	for userID, raw := range gc.VoiceStates {
		channelID, err := discordjson.ChannelIDOf(raw)
		if err != nil {
			return xerrors.Errorf("read voice channel: %w", err)
		}
		if channelID == 0 {
			continue
		}
		// GUILD_CREATE voice states may omit the member; without the presence
		// intent Discord still lists every voice-connected member in members.
		isBot := discordjson.VoiceStateIsBot(raw)
		if isBot == nil {
			if member, ok := gc.Members[userID]; ok {
				isBot = discordjson.MemberIsBot(member)
			}
		}
		baseline.Occupants = append(baseline.Occupants, state.VoiceOccupant{
			UserID:    userID,
			ChannelID: channelID,
			IsBot:     isBot,
		})
	}

	if err := c.db.ReplaceGuildVoiceStates(ctx, baseline); err != nil {
		return xerrors.Errorf("replace voice baseline: %w", err)
	}
	return nil
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

	// Removed or unavailable either way: nothing State holds about the guild's
	// voice presence can be vouched for any more.
	if c.TracksVoice(gc.ID) {
		eg.Go(func() error {
			if err := c.db.ClearGuildVoiceStates(ctx, gc.ID); err != nil {
				return &VoiceStateError{GuildID: gc.ID, Err: xerrors.Errorf("clear voice states: %w", err)}
			}
			return nil
		})
	}

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
