package api

import (
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuildMember(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var m []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		m = t.Get(s.fmtGuildMemberKey(guildParam(p), memberParam(p))).MustGet()
		return nil
	})
	if err != nil {
		return xerrors.Errorf("failed to transact member: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return writeTerm(w, m)
}

func (s *Server) getGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var raws []fdb.KeyValue

	pre, _ := fdb.PrefixRange(s.fmtGuildMemberPrefix(guildParam(p)))
	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return xerrors.Errorf("failed to transact members: %w", err)
	}

	return writeTerms(w, raws)
}

func memberParam(p httprouter.Params) int64 {
	m := p.ByName("member")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return mi
}
