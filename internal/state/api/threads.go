package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getThread(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	threadID, err := threadParam(p)
	if err != nil {
		return xerrors.Errorf("thread param: %w", err)
	}
	c, err := s.db.GetThread(r.Context(), threadID)
	if err != nil {
		return xerrors.Errorf("read thread: %w", err)
	}

	if c == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, c)
}

func threadParam(p httprouter.Params) (int64, error) {
	c := p.ByName("thread")
	ci, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		return 0, ErrInvalidArgument
	}

	return ci, nil
}

func (s *Server) getThreads(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	cs, err := s.db.GetThreads(r.Context())
	if err != nil {
		return xerrors.Errorf("read threads: %w", err)
	}

	return s.writeTerms(w, cs)
}

func (s *Server) getGuildThreads(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	cs, err := s.db.GetGuildThreads(r.Context(), guild)
	if err != nil {
		return xerrors.Errorf("read guild threads: %w", err)
	}

	return s.writeTerms(w, cs)
}

func (s *Server) getChannelThreads(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	channel, err := channelParam(p)
	if err != nil {
		return xerrors.Errorf("read channel param: %w", err)
	}
	cs, err := s.db.GetChannelThreads(r.Context(), channel)
	if err != nil {
		return xerrors.Errorf("read channel threads: %w", err)
	}

	return s.writeTerms(w, cs)
}
