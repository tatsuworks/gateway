package discordetf

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeRole(buf []byte) (*discord.Role, error) {
	var (
		r   = &discord.Role{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	r.ID, r.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return nil, xerrors.Errorf("read id: %w", err)
	}

	d.reset()
	r.GuildID, err = d.guildIDFromMap()
	if err != nil {
		return nil, xerrors.Errorf("read guild id: %w", err)
	}

	return r, err
}

func (_ decoder) DecodeRoleDelete(buf []byte) (*discord.RoleDelete, error) {
	var (
		r   = &discord.RoleDelete{}
		d   = &etfDecoder{buf: buf}
		err error
	)

	r.ID, err = d.idFromMap("role_id")
	if err != nil {
		return nil, xerrors.Errorf("read role id: %w", err)
	}
	d.reset()

	r.GuildID, err = d.idFromMap("guild_id")
	if err != nil {
		return nil, xerrors.Errorf("read guild id: %w", err)
	}

	return r, nil
}
