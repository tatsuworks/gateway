package api

import (
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Server) getMember(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var m []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		m = t.Get(s.fmtMemberKey(guildParam(p), memberParam(p))).MustGet()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to transact channel")
	}

	if m == nil {
		return errors.New("guild not found")
	}

	return writeTerm(w, m)
}

func (s *Server) getMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var raws []fdb.KeyValue

	pre, _ := fdb.PrefixRange(s.fmtMemberKey(guildParam(p), 0))
	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to read members")
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
