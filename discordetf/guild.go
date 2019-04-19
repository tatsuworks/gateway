package discordetf

import (
	"bytes"

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
		d        = &decoder{buf: buf}
		gBuf     = new(bytes.Buffer)
		gc       = &GuildCreate{}
		mapStart = d.off
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return gc, errors.Wrap(err, "failed to verify guild map byte")
	}

	left := d.readMapLen()

	mapEnd := d.off
	_, err = gBuf.Write(d.buf[mapStart:mapEnd])
	if err != nil {
		return gc, err
	}

	for ; left > 0; left-- {
		start := d.off

		l, err := d.readAtomWithTag()
		if err != nil {
			return gc, errors.Wrap(err, "failed to read guild map key")
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "channels":
			//fmt.Println("channels")
			gc.Channels, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild channels")
			}

		case "emojis":
			//fmt.Println("emoji")
			gc.Emojis, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild emojis")
			}

		case "members":
			//fmt.Println("members")
			gc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild members")
			}

		case "presences":
			//fmt.Println("presences")
			gc.Presences, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild presences")
			}

		case "roles":
			//fmt.Println("roles")
			gc.Roles, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild roles")
			}

		case "voice_states":
			//fmt.Println("voice_states")
			gc.VoiceStates, err = d.readListIntoMapByID()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild voice states")
			}

		case "id":
			gc.Id, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return gc, errors.Wrap(err, "failed to read guild id")
			}

			_, err = gBuf.Write(d.buf[start:d.off])
			if err != nil {
				return gc, errors.Wrap(err, "failed to write guild data to buf")
			}

		default:
			//fmt.Println("default")
			err := d.readTerm()
			if err != nil {
				return gc, err
			}
			_, err = gBuf.Write(d.buf[start:d.off])
			if err != nil {
				return gc, err
			}
		}
	}

	gc.Guild = gBuf.Bytes()

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
