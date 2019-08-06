package api

import (
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuildRole(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var ro []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		ro = t.Get(s.fmtGuildRoleKey(guildParam(p), roleParam(p))).MustGet()

		return nil
	})
	if err != nil {
		return xerrors.Errorf("failed to transact role: %w", err)
	}

	if ro == nil {
		return ErrNotFound
	}

	return writeTerm(w, ro)
}

func (s *Server) getGuildRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var raws []fdb.KeyValue

	pre, _ := fdb.PrefixRange(s.fmtGuildRolePrefix(guildParam(p)))
	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return xerrors.Errorf("failed to read roles: %w", err)
	}

	return writeTerms(w, raws)
}

func roleParam(p httprouter.Params) int64 {
	r := p.ByName("role")
	ri, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ri
}
