package discordetf

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeChannel(buf []byte) (*discord.Channel, error) {
	var (
		ch  = &discord.Channel{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	ch.ID, ch.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("extract id: %w", err)
	}

	d.reset()
	ch.GuildID, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("extract guild_id: %w", err)
	}

	return ch, err
}

func (_ decoder) DecodeVoiceState(buf []byte) (*discord.VoiceState, error) {
	var (
		vs  = &discord.VoiceState{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	vs.UserID, vs.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("extract user_id: %w", err)
	}

	d.reset()
	vs.GuildID, err = d.idFromMap("guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild_id: %w", err)
	}

	return vs, err
}
