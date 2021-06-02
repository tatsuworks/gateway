package discordetf

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeThread(buf []byte) (*discord.Thread, error) {
	var (
		th  = &discord.Thread{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	th.ID, th.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("extract id: %w", err)
	}

	d.reset()
	th.GuildID, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("extract guild_id: %w", err)
	}

	d.reset()
	th.OwnerID, err = d.idFromMap("owner_id")
	if err != nil {
		return nil, xerrors.Errorf("read owner id: %w", err)
	}

	d.reset()
	th.ParentID, err = d.idFromMap("parent_id")
	if err != nil {
		return nil, xerrors.Errorf("read parent id: %w", err)
	}

	return th, err
}
