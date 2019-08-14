package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuildRole(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	ro, err := s.db.GetGuildRole(guildParam(p), roleParam(p))
	if err != nil {
		return xerrors.Errorf("failed to read role: %w", err)
	}

	if ro == nil {
		return ErrNotFound
	}

	return writeTerm(w, ro)
}

func (s *Server) getGuildRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	ros, err := s.db.GetGuildRoles(guildParam(p))
	if err != nil {
		return xerrors.Errorf("failed to read roles: %w", err)
	}

	return writeTerms(w, ros)
}

func roleParam(p httprouter.Params) int64 {
	r := p.ByName("role")
	ri, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ri
}
