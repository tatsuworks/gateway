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
		return xerrors.Errorf("read role: %w", err)
	}

	if ro == nil {
		return ErrNotFound
	}

	return writeTerm(w, ro)
}

func (s *Server) getGuildRoles(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	if len(r.URL.Query()["id"]) > 0 {
		return s.getGuildRoleSlice(w, r, p)
	}

	ros, err := s.db.GetGuildRoles(guildParam(p))
	if err != nil {
		return xerrors.Errorf("read roles: %w", err)
	}

	return writeTerms(w, ros)
}

func (s *Server) getGuildRoleSlice(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var (
		g   = guildParam(p)
		rs  = r.URL.Query()["id"]
		ros = make([][]byte, len(rs))
	)

	for i, e := range rs {
		rr, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return xerrors.Errorf("parse role id: %w", err)
		}

		ros[i], err = s.db.GetGuildRole(g, rr)
		if err != nil {
			return xerrors.Errorf("get role: %w", err)
		}
	}

	return writeTermsRaw(w, ros)
}

func roleParam(p httprouter.Params) int64 {
	r := p.ByName("role")
	ri, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ri
}
