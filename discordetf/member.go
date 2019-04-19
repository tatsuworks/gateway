package discordetf

import (
	"github.com/pkg/errors"
)

type MemberChunk struct {
	Guild   int64
	Members map[int64][]byte
}

func DecodeMemberChunk(buf []byte) (*MemberChunk, error) {
	var (
		mc = &MemberChunk{}
		d  = &decoder{buf: buf}
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return mc, errors.WithStack(err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return mc, err
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "guild_id":
			mc.Guild, err = d.readSmallBigWithTagToInt64()
		case "members":
			mc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return mc, errors.WithStack(err)
			}
		default:
			return mc, errors.Errorf("unknown key found in member chunk: %s", key)
		}

	}

	return mc, nil
}

type Member struct {
	Id    int64
	Guild int64
	Raw   []byte
}

func DecodeMember(buf []byte) (*Member, error) {
	var (
		m   = &Member{}
		d   = &decoder{buf: buf}
		err error
	)

	m.Id, m.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return m, errors.WithStack(err)
	}

	d.reset()
	m.Guild, err = d.guildIDFromMap()
	if err != nil {
		return m, errors.WithStack(err)
	}

	return m, err
}

type Presence struct {
	Id    int64
	Guild int64
	Raw   []byte
}

func DecodePresence(buf []byte) (*Presence, error) {
	var (
		p   = &Presence{}
		d   = &decoder{buf: buf}
		err error
	)

	p.Id, p.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return p, errors.WithStack(err)
	}

	d.reset()
	p.Guild, err = d.guildIDFromMap()
	if err != nil {
		return p, errors.WithStack(err)
	}

	return p, err
}

//type PlayedPresence struct {
//	Id   int64
//	Game string
//}
//
//func DecodePlayedPresence(buf []byte) (*Presence, error) {
//	var (
//		p   = &Presence{}
//		d   = &decoder{buf: buf}
//		err error
//	)
//
//	err = d.readUntilData()
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to skip to data")
//	}
//
//	err = d.checkByte(ettMap)
//	if err != nil {
//		return nil, errors.Wrap(err, "failed to verify map byte")
//	}
//
//	arity := d.readMapLen()
//	for ; arity > 0; arity-- {
//		l, err := d.readAtomWithTag()
//	}
//
//	return p, err
//}
