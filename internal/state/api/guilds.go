package api

import (
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var g []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		g = t.Get(s.fmtGuildKey(guildParam(p))).MustGet()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to transact channel")
	}

	if g == nil {
		return ErrNotFound
	}

	return writeTerm(w, g)
}

func guildParam(p httprouter.Params) int64 {
	c := p.ByName("guild")
	ci, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ci
}
