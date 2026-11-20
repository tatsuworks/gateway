package handler

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
	"github.com/tatsuworks/gateway/internal/state"
)

func (c *Client) ChannelCreate(ctx context.Context, d []byte) error {
	ch, err := c.enc.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.SetChannel(ctx, ch.GuildID, ch.ID, ch.Raw)
	if err != nil {
		return xerrors.Errorf("set channel: %w", err)
	}

	return nil
}

func (c *Client) ChannelDelete(ctx context.Context, d []byte) error {
	ch, err := c.enc.DecodeChannel(d)
	if err != nil {
		return err
	}

	err = c.db.DeleteChannel(ctx, ch.GuildID, ch.ID)
	if err != nil {
		return xerrors.Errorf("delete channel: %w", err)
	}

	// A deleted channel holds nobody; State must not keep reporting them there.
	if c.TracksVoice(ch.GuildID) {
		if err := c.db.DeleteChannelVoiceStates(ctx, ch.GuildID, ch.ID); err != nil {
			return &VoiceStateError{GuildID: ch.GuildID, Err: xerrors.Errorf("delete channel voice states: %w", err)}
		}
	}

	return nil
}

func (c *Client) VoiceStateUpdate(ctx context.Context, d []byte) error {
	return c.voiceStateUpdate(ctx, &discord.Event{T: "VOICE_STATE_UPDATE", D: d})
}

func (c *Client) voiceStateUpdate(ctx context.Context, e *discord.Event) error {
	vs, err := c.enc.DecodeVoiceState(e.D)
	if err != nil {
		return err
	}

	// Voice presence is stored only for attendance guilds; everywhere else the
	// update stays the no-op it has always been.
	if !c.TracksVoice(vs.GuildID) {
		return nil
	}

	err = c.db.ApplyVoiceState(ctx, state.VoiceStateChange{
		GuildID:    vs.GuildID,
		UserID:     vs.UserID,
		ChannelID:  vs.ChannelID,
		IsBot:      vs.IsBot,
		ChangedAt:  e.Receipt.ReceivedAt,
		DiscordSeq: e.S,
		Raw:        vs.Raw,
	})
	if err != nil {
		return &VoiceStateError{GuildID: vs.GuildID, Err: xerrors.Errorf("set voice state: %w", err)}
	}

	return nil
}
