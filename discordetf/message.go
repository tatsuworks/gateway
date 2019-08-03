package discordetf

import (
	"github.com/pkg/errors"
	"golang.org/x/xerrors"
)

type Message struct {
	Id      int64
	Channel int64
	Raw     []byte
}

func DecodeMessage(buf []byte) (*Message, error) {
	var (
		m   = &Message{}
		d   = &decoder{buf: buf}
		err error
	)

	m.Id, m.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return m, errors.WithStack(err)
	}

	d.reset()
	m.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return m, errors.WithStack(err)
	}

	return m, err
}

type MessageReaction struct {
	Message int64
	Channel int64
	User    int64
	Name    interface{}
	Raw     []byte
}

func DecodeMessageReaction(buf []byte) (*MessageReaction, error) {
	var (
		mr  = &MessageReaction{}
		d   = &decoder{buf: buf}
		err error
	)

	mr.Message, err = d.idFromMap("message_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to read message id: %w", err)
	}
	d.reset()

	mr.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to read channel id: %w", err)
	}
	d.reset()

	mr.User, err = d.idFromMap("user_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to read user id: %w", err)
	}
	d.reset()

	err = d.readUntilKey("emoji")
	if err != nil {
		return nil, xerrors.Errorf("failed to read until emoji: %w", err)
	}

	mr.Name, err = d.readEmojiID()
	if err != nil {
		return nil, xerrors.Errorf("failed to read emoji id: %w", err)
	}

	mr.Raw = buf

	return mr, err
}

type MessageReactionRemoveAll struct {
	Message int64
	Channel int64
	User    int64
}

func DecodeMessageReactionRemoveAll(buf []byte) (*MessageReactionRemoveAll, error) {
	var (
		mr  = &MessageReactionRemoveAll{}
		d   = &decoder{buf: buf}
		err error
	)

	mr.Message, err = d.idFromMap("message_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to read message id: %w", err)
	}
	d.reset()

	mr.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to read channel id: %w", err)
	}
	d.reset()

	mr.User, err = d.idFromMap("user_id")
	if err != nil {
		return nil, xerrors.Errorf("failed to read user id: %w", err)
	}

	return mr, err
}
