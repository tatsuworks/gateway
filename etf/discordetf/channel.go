package discordetf

import (
	"github.com/pkg/errors"
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
		return ch, errors.WithStack(err)
	}

	d.reset()
	ch.Guild, err = d.guildIDFromMap()
	if err != nil {
		return ch, errors.WithStack(err)
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
		return vs, errors.WithStack(err)
	}

	d.reset()
	vs.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return vs, errors.WithStack(err)
	}

	return vs, err
}
