package discordetf

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeMessage(buf []byte) (*discord.Message, error) {
	var (
		m   = &discord.Message{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	m.ID, m.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("read id: %w", err)
	}

	d.reset()
	m.ChannelID, err = d.idFromMap("channel_id")
	if err != nil {
		return nil, xerrors.Errorf("read channel id: %w", err)
	}

	return m, nil
}

func (_ decoder) DecodeMessageReaction(buf []byte) (*discord.MessageReaction, error) {
	var (
		mr  = &discord.MessageReaction{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	mr.MessageID, err = d.idFromMap("message_id")
	if err != nil {
		return nil, xerrors.Errorf("read message id: %w", err)
	}
	d.reset()

	mr.ChannelID, err = d.idFromMap("channel_id")
	if err != nil {
		return nil, xerrors.Errorf("read channel id: %w", err)
	}
	d.reset()

	mr.UserID, err = d.idFromMap("user_id")
	if err != nil {
		return nil, xerrors.Errorf("read user id: %w", err)
	}
	d.reset()

	err = d.readUntilKey("emoji")
	if err != nil {
		return nil, xerrors.Errorf("read until emoji: %w", err)
	}

	mr.Name, err = d.readEmojiID()
	if err != nil {
		return nil, xerrors.Errorf("read emoji id: %w", err)
	}

	mr.Raw = buf

	return mr, nil
}

func (_ decoder) DecodeMessageReactionRemoveAll(buf []byte) (*discord.MessageReactionRemoveAll, error) {
	var (
		mr  = &discord.MessageReactionRemoveAll{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	mr.MessageID, err = d.idFromMap("message_id")
	if err != nil {
		return nil, xerrors.Errorf("read message id: %w", err)
	}
	d.reset()

	mr.ChannelID, err = d.idFromMap("channel_id")
	if err != nil {
		return nil, xerrors.Errorf("read channel id: %w", err)
	}
	d.reset()

	mr.UserID, err = d.idFromMap("user_id")
	if err != nil {
		return nil, xerrors.Errorf("read user id: %w", err)
	}

	return mr, nil
}
