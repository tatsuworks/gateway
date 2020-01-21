package discordetf

import "golang.org/x/xerrors"

type GuildEmojisUpdate struct {
	GuildID int64
	Emojis  map[int64][]byte
}

func DecodeGuildEmojisUpdate(buf []byte) (*GuildEmojisUpdate, error) {
	var (
		eu = &GuildEmojisUpdate{}
		d  = &decoder{buf: buf}
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
