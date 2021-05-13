package discordetf

import (
	"encoding/binary"

	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeGuildCreate(buf []byte) (*discord.GuildCreate, error) {
	var (
		d     = &etfDecoder{buf: buf}
		gBuf  = []byte{116, 0, 0, 0, 0}
		gKeys uint32
		gc    = &discord.GuildCreate{}
		id    int64
		err   error
	)

	id, err = d.idFromMap("id")
	if err != nil {
		return nil, xerrors.Errorf("find guild id: %w", err)
	}
	d.reset()

	err = d.checkByte(ettMap)
	if err != nil {
		return nil, xerrors.Errorf("verify map byte: %w", err)
	}

	left := d.readMapLen()
	for ; left > 0; left-- {
		start := d.off

		l, err := d.readAtomWithTag()
		if err != nil {
			return nil, xerrors.Errorf("read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "channels":
			gc.Channels, err = d.readListIntoMapByIDFixGuildID(id)
			if err != nil {
				return nil, xerrors.Errorf("read channels: %w", err)
			}

		case "emojis":
			gc.Emojis, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("read emojis: %w", err)
			}

		case "members":
			gc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("read members: %w", err)
			}

		case "presences":
			gc.Presences, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("read presences: %w", err)
			}

		case "roles":
			gc.Roles, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("read roles: %w", err)
			}

		case "voice_states":
			gc.VoiceStates, err = d.readListIntoMapByIDFixGuildID(id)
			if err != nil {
				return nil, xerrors.Errorf("read voice states: %w", err)
			}

		case "id":
			gc.ID, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return nil, xerrors.Errorf("read id: %w", err)
			}

			gBuf = append(gBuf, d.buf[start:d.off]...)
			gKeys++

		case "member_count":
			gc.MemberCount, err = d.readInteger()
			if err != nil {
				return nil, xerrors.Errorf("read member_count: %w", err)
			}

			gBuf = append(gBuf, d.buf[start:d.off]...)
			gKeys++

		case "threads":
			gc.Threads, err = d.readListIntoMapByIDFixGuildID(id)
			if err != nil {
				return nil, xerrors.Errorf("read threads: %w", err)
			}

		default:
			err := d.readTerm()
			if err != nil {
				return nil, err
			}

			gBuf = append(gBuf, d.buf[start:d.off]...)
			gKeys++
		}
	}

	// fix length
	binary.BigEndian.PutUint32(gBuf[1:5], gKeys)
	gc.Raw = gBuf

	return gc, nil
}

func (_ decoder) DecodeGuildBan(buf []byte) (*discord.GuildBan, error) {
	var (
		gb  = &discord.GuildBan{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	gb.UserID, gb.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("read user id: %w", err)
	}

	d.reset()
	gb.GuildID, err = d.idFromMap("guild_id")
	if err != nil {
		return nil, xerrors.Errorf("read guild id: %w", err)
	}

	return gb, err
}
