package state

import (
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/gateway/discordetf"
)

func (c *Client) RoleCreate(d []byte) error {
	rc, err := discordetf.DecodeRole(d)
	if err != nil {
		return err
	}

	return c.Transact(func(t fdb.Transaction) error {
		t.Set(c.fmtRoleKey(rc.Guild, rc.Id), rc.Raw)
		return nil
	})
}

func (c *Client) RoleDelete(d []byte) error {
	rc, err := discordetf.DecodeRoleDelete(d)
	if err != nil {
		return err
	}
	return c.Transact(func(t fdb.Transaction) error {
		t.Clear(c.fmtRoleKey(rc.Guild, rc.Id))
		return nil
	})
}
