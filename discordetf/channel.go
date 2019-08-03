package discordetf

import (
	"golang.org/x/xerrors"
)

type Channel struct {
	Id    int64
	Guild int64
	Raw   []byte
}

func DecodeChannel(buf []byte) (*Channel, error) {
	var (
		ch  = &Channel{}
		d   = &decoder{buf: buf}
		err error
	)

	ch.Id, ch.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("failed to extract id: %w", err)
	}

	d.reset()
	ch.Guild, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("failed to extract guild_id: %w", err)
	}

	return ch, err
}

type VoiceState struct {
	User    int64
	Channel int64
	Raw     []byte
}

func DecodeVoiceState(buf []byte) (*VoiceState, error) {
	var (
		vs  = &VoiceState{}
		d   = &decoder{buf: buf}
		err error
	)

	vs.User, vs.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("failed to extract user_id: %w", err)
	}

	d.reset()
	vs.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to extract channel_id: %w", err)
	}

	return vs, err
}
