package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	g, err := s.db.GetGuild(r.Context(), guildParam(p))
	if err != nil && !xerrors.Is(err, ErrNotFound) {
		return xerrors.Errorf("read guild: %w", err)
	}

	return s.writeTerm(w, g)
}

func guildParam(p httprouter.Params) int64 {
	c := p.ByName("guild")
	ci, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ci
}
