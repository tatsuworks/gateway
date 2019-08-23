package discordetf

import (
	"golang.org/x/xerrors"
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
		return nil, xerrors.Errorf("failed to verify map byte: %w", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return nil, xerrors.Errorf("failed to read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "guild_id":
			mc.Guild, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return nil, xerrors.Errorf("failed to extract guild_id from map: %w", err)
			}

		case "members":
			mc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("failed to extract members list from maplenbytes: %w", err)
			}

		default:
			return nil, xerrors.Errorf("unknown key found in member chunk: %s", key)
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
		return nil, xerrors.Errorf("failed to read id: %w", err)
	}

	d.reset()
	m.Guild, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("failed to read guild id: %w", err)
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
		return nil, xerrors.Errorf("failed to read id: %w", err)
	}

	d.reset()
	p.Guild, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("failed to read guild id: %w", err)
	}

	return p, err
}

type PlayedPresence struct {
	UserID int64
	Game   string
}

func DecodePlayedPresence(buf []byte) (*PlayedPresence, error) {
	var (
		p   = &PlayedPresence{}
		d   = &decoder{buf: buf}
		err error
	)

	err = d.checkByte(ettMap)
	if err != nil {
		return nil, xerrors.Errorf("failed to check starting byte: %w", err)
	}

	left := d.readMapLen()
	for ; left > 0; left-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return nil, xerrors.Errorf("failed to read map key: %w", err)
		}
		key := string(d.buf[d.off-l : d.off])

		switch key {
		case "user":
			p.UserID, err = d.idFromMap("id")
			if err != nil {
				return nil, xerrors.Errorf("failed to extract user id: %w", err)
			}

			continue

		case "game":
			p.Game, err = d.stringFromMap("name")
			if err != nil {
				return nil, xerrors.Errorf("failed to extract game name: %w", err)
			}

			continue
		}

		err = d.readTerm()
		if err != nil {
			return nil, xerrors.Errorf("failed to read term: %w", err)
		}
	}

	return p, nil
}
