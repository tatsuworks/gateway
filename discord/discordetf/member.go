package discordetf

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeMemberChunk(buf []byte) (*discord.MemberChunk, error) {
	var (
		mc = &discord.MemberChunk{}
		d  = &etfDecoder{buf: buf}
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return nil, xerrors.Errorf("verify map byte: %w", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return nil, xerrors.Errorf("read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "guild_id":
			mc.GuildID, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return nil, xerrors.Errorf("extract guild_id from map: %w", err)
			}

		case "members":
			mc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("extract members list from map: %w", err)
			}

		default:
		}

	}

	return mc, nil
}

func (_ decoder) DecodeMember(buf []byte) (*discord.Member, error) {
	var (
		m   = &discord.Member{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	m.ID, m.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("read id: %w", err)
	}

	d.reset()
	m.GuildID, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("read guild id: %w", err)
	}

	return m, err
}

func (_ decoder) DecodePresence(buf []byte) (*discord.Presence, error) {
	var (
		p   = &discord.Presence{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	p.ID, p.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("read id: %w", err)
	}

	d.reset()
	p.GuildID, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("read guild id: %w", err)
	}

	return p, err
}
