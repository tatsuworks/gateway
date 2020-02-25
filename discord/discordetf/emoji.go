package discordetf

import (
	"github.com/tatsuworks/gateway/discord"
	"golang.org/x/xerrors"
)

func (_ decoder) DecodeGuildEmojisUpdate(buf []byte) (*discord.GuildEmojisUpdate, error) {
	var (
		eu = &discord.GuildEmojisUpdate{}
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
			eu.GuildID, err = d.readInteger()
			if err != nil {
				return nil, xerrors.Errorf("extract guild_id from map: %w", err)
			}

		case "emojis":
			eu.Emojis, err = d.readListIntoMapByID()
			if err != nil {
				return nil, xerrors.Errorf("extract emojis list from map: %w", err)
			}

		default:
		}

	}

	return eu, nil
}
