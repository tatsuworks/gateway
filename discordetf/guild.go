package discordetf

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

type GuildCreate struct {
	Id          int64
	Guild       []byte
	Channels    map[int64][]byte
	Emojis      map[int64][]byte
	Members     map[int64][]byte
	Presences   map[int64][]byte
	Roles       map[int64][]byte
	VoiceStates map[int64][]byte
}

func DecodeGuildCreate(buf []byte) (*GuildCreate, error) {
	var (
		d     = &decoder{buf: buf}
		gBuf  = []byte{}
		gKeys uint32
		gc    = &GuildCreate{}
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return gc, errors.Wrap(err, "failed to verify guild map byte")
	}

	left := d.readMapLen()

	for ; left > 0; left-- {
		start := d.off

		l, err := d.readAtomWithTag()
		if err != nil {
			return gc, errors.Wrap(err, "failed to read guild map key")
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "channels":
			gc.Channels, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild channels")
			}

		case "emojis":
			gc.Emojis, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild emojis")
			}

		case "members":
			gc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild members")
			}

		case "presences":
			gc.Presences, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild presences")
			}

		case "roles":
			gc.Roles, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild roles")
			}

		case "voice_states":
			gc.VoiceStates, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild voice states")
			}

		case "id":
			gc.Id, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild id")
			}

			gBuf = append(gBuf, d.buf[start:d.off]...)
			gKeys++

		default:
			err := d.readTerm()
			if err != nil {
				return gc, err
			}

			gBuf = append(gBuf, d.buf[start:d.off]...)
			gKeys++
		}
	}

	// fix length
	binary.BigEndian.PutUint32(gBuf[1:5], gKeys)
	gc.Guild = gBuf

	return gc, nil
}

type GuildBan struct {
	User  int64
	Guild int64
	Raw   []byte
}

func DecodeGuildBan(buf []byte) (*GuildBan, error) {
	var (
		gb  = &GuildBan{}
		d   = &decoder{buf: buf}
		err error
	)

	gb.User, gb.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return gb, errors.WithStack(err)
	}

	d.reset()
	gb.Guild, err = d.idFromMap("guild_id")
	if err != nil {
		return gb, errors.WithStack(err)
	}

	return gb, err
}
