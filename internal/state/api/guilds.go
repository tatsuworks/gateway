package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	g, err := s.db.GetGuild(r.Context(), guildParam(p))
	if err != nil {
		return xerrors.Errorf("read guild: %w", err)
	}

	if g == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, g)
}

func (s *Server) getGuildCount(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	count, err := s.db.GetGuildCount(r.Context())
	if err != nil {
		return xerrors.Errorf("read guild count: %w", err)
	}

	return s.writeTerm(w, []byte(strconv.Itoa(count)))
}

func guildParam(p httprouter.Params) int64 {
	c := p.ByName("guild")
	ci, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ci
}
