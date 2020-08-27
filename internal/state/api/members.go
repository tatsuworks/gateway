package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getGuildMember(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	m, err := s.db.GetGuildMember(r.Context(), guildParam(p), memberParam(p))
	if err != nil {
		return xerrors.Errorf("read member: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, m)
}

func (s *Server) getGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	if len(r.URL.Query()["id"]) > 0 {
		return s.getGuildMemberSlice(w, r, p)
	}

	if r.URL.Query().Get("query") != "" {
		return s.searchGuildMembers(w, r, p)
	}

	ms, err := s.db.GetGuildMembers(r.Context(), guildParam(p))
	if err != nil {
		return xerrors.Errorf("read members: %w", err)
	}

	return s.writeTerms(w, ms)
}

func (s *Server) getGuildMemberSlice(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var (
		g   = guildParam(p)
		ms  = r.URL.Query()["id"]
		mrs = make([][]byte, 0, len(ms))
	)

	for _, e := range ms {
		mr, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			return xerrors.Errorf("parse member id: %w", err)
		}

		mbmr, err := s.db.GetGuildMember(r.Context(), g, mr)
		if err != nil {
			if xerrors.Is(err, ErrNotFound) {
				mbmr, _ = json.Marshal(EmptyObj{Id: e, IsEmpty: true})
			} else {
				return xerrors.Errorf("get member: %w", err)
			}
		}

		mrs = append(mrs, mbmr)
	}

	return s.writeTerms(w, mrs)
}

func (s *Server) searchGuildMembers(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var (
		ctx   = r.Context()
		g     = guildParam(p)
		query = r.URL.Query().Get("query")
	)

	ms, err := s.db.SearchGuildMembers(ctx, g, query)
	if err != nil {
		return xerrors.Errorf("search members: %w", err)
	}

	return s.writeTerms(w, ms)
}

func memberParam(p httprouter.Params) int64 {
	m := p.ByName("member")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return mi
}

func userParam(p httprouter.Params) int64 {
	u := p.ByName("user")
	ui, err := strconv.ParseInt(u, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ui
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	m, err := s.db.GetUser(r.Context(), userParam(p))
	if err != nil {
		return xerrors.Errorf("read user: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, m)
}
