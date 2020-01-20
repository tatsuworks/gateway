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
		return xerrors.Errorf("read member: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return writeTerm(w, m)
}

func (s *Server) getGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	if len(r.URL.Query()["id"]) > 0 {
		return s.getGuildMemberSlice(w, r, p)
	}

	ms, err := s.db.GetGuildMembers(guildParam(p))
	if err != nil {
		return xerrors.Errorf("read members: %w", err)
	}

	return writeTerms(w, ms)
}

func (s *Server) getGuildMemberSlice(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var (
		g   = guildParam(p)
		ms  = r.URL.Query()["id"]
		mrs = make([][]byte, len(ms))
	)

	for i, e := range ms {
		mr, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return xerrors.Errorf("parse member id: %w", err)
		}

		mrs[i], err = s.db.GetGuildMember(g, mr)
		if err != nil {
			return xerrors.Errorf("get member: %w", err)
		}
	}

	return writeTermsRaw(w, mrs)
}

func memberParam(p httprouter.Params) int64 {
	m := p.ByName("member")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return mi
}
