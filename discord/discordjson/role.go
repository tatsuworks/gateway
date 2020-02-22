package discordjson

import (
	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

func (_ decoder) DecodeRole(buf []byte) (*discord.Role, error) {
	var (
		r   discord.Role
		err error
	)

	r.ID, err = idFromNestedObject(buf, "role")
	if err != nil {
		return nil, xerrors.Errorf("extract role id: %w", err)
	}

	r.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	r.Raw = buf
	return &r, nil
}
func (_ decoder) DecodeRoleDelete(buf []byte) (*discord.RoleDelete, error) {
	var (
		r   discord.RoleDelete
		err error
	)

	r.ID, err = snowflakeFromObject(buf, "role_id")
	if err != nil {
		return nil, xerrors.Errorf("extract role id: %w", err)
	}

	r.GuildID, err = snowflakeFromObject(buf, "guild_id")
	if err != nil {
		return nil, xerrors.Errorf("extract guild id: %w", err)
	}

	return &r, nil
}
