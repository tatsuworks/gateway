package api

import (
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Server) getRole(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var ro []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		ro = t.Get(s.fmtMessageKey(guildParam(p), roleParam(p))).MustGet()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to transact role")
	}

	if ro == nil {
		return errors.New("role not found")
	}

	return writeTerm(w, ro)
}

func (s *Server) getRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var raws []fdb.KeyValue

	pre, _ := fdb.PrefixRange(s.fmtRolesKey(guildParam(p)))
	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to read roles")
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
