package api

import (
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Server) getMessage(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var m []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		m = t.Get(s.fmtMessageKey(channelParam(p), messageParam(p))).MustGet()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to transact message")
	}

	if m == nil {
		return errors.New("message not found")
	}

	return writeTerm(w, m)
}

func messageParam(p httprouter.Params) int64 {
	m := p.ByName("message")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return mi
}
