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
		return gc, errors.Wrap(err, "failed to verify list byte")
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
			return gc, err
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "channels":
			//fmt.Println("channels")
			gc.Channels, err = d.readListIntoMapByID()
			if err != nil {
				return gc, err
			}

		case "emojis":
			//fmt.Println("emoji")
			gc.Emojis, err = d.readListIntoMapByID()
			if err != nil {
				return gc, err
			}

		case "members":
			//fmt.Println("members")
			gc.Members, err = d.readListIntoMapByID()
			if err != nil {
				return gc, err
			}

		case "presences":
			//fmt.Println("presences")
			gc.Presences, err = d.readListIntoMapByID()
			if err != nil {
				return gc, err
			}

		case "roles":
			//fmt.Println("roles")
			gc.Roles, err = d.readListIntoMapByID()
			if err != nil {
				return gc, err
			}

		case "voice_states":
			//fmt.Println("voice_states")
			gc.VoiceStates, err = d.readListIntoMapByID()
			if err != nil {
				return gc, err
			}

		case "id":
			gc.Id, err = d.smallBigWithTagToInt64()
			if err != nil {
				return gc, err
			}

			_, err = gBuf.Write(d.buf[start:d.off])
			if err != nil {
				return gc, err
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
