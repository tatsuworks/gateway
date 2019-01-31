package discordetf

import (
	"github.com/pkg/errors"
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
		return mr, errors.Wrap(err, "failed to get message_id from reaction")
	}
	d.reset()

	mr.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return mr, errors.Wrap(err, "failed to get channel_id from reaction")
	}
	d.reset()

	mr.User, err = d.idFromMap("user_id")
	if err != nil {
		return mr, errors.Wrap(err, "failed to get user_id from reaction")
	}
	d.reset()

	err = d.readUntilKey("emoji")
	if err != nil {
		return mr, errors.Wrap(err, "failed to read until emoji key")
	}

	mr.Name, err = d.readEmojiID()
	if err != nil {
		return mr, errors.Wrap(err, "failed to read emoji id")
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
		return mr, errors.Wrap(err, "failed to get message_id from reaction remove all")
	}
	d.reset()

	mr.Channel, err = d.idFromMap("channel_id")
	if err != nil {
		return mr, errors.Wrap(err, "failed to get channel_id from reaction remove all")
	}
	d.reset()

	mr.User, err = d.idFromMap("user_id")
	if err != nil {
		return mr, errors.Wrap(err, "failed to get user_id from reaction remove all")
	}

	return mr, err
}
