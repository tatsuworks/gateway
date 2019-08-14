package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuildMember(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	m, err := s.db.GetGuildMember(guildParam(p), memberParam(p))
	if err != nil {
		return xerrors.Errorf("failed to read member: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return writeTerm(w, m)
}

func (s *Server) getGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	ms, err := s.db.GetGuildMembers(guildParam(p))
	if err != nil {
		return xerrors.Errorf("failed to read members: %w", err)
	}

	return writeTerms(w, ms)
}

func memberParam(p httprouter.Params) int64 {
	m := p.ByName("member")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return mi
}
