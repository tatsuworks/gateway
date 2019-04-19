package discordetf

import "github.com/pkg/errors"

type Role struct {
	Id    int64
	Guild int64
	Raw   []byte
}

func DecodeRole(buf []byte) (*Role, error) {
	var (
		r   = &Role{}
		d   = &decoder{buf: buf}
		err error
	)

	r.Id, r.Raw, err = d.readMapWithIDIntoSlice()
	if err != nil {
		return r, errors.WithStack(err)
	}

	d.reset()
	r.Guild, err = d.guildIDFromMap()
	if err != nil {
		return r, errors.WithStack(err)
	}

	return r, err
}

type RoleDelete struct {
	Id    int64
	Guild int64
}

func DecodeRoleDelete(buf []byte) (*RoleDelete, error) {
	var (
		r   = &RoleDelete{}
		d   = &decoder{buf: buf}
		err error
	)

	r.Id, err = d.idFromMap("role_id")
	if err != nil {
		return r, errors.WithStack(err)
	}
	d.reset()

	r.Guild, err = d.idFromMap("guild_id")
	if err != nil {
		return r, errors.WithStack(err)
	}

	return r, err
}
