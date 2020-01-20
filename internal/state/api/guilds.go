package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	g, err := s.db.GetGuild(guildParam(p))
	if err != nil {
		return xerrors.Errorf("read guild: %w", err)
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
