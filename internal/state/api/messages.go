package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getChannelMessage(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	m, err := s.db.GetChannelMessage(channelParam(p), messageParam(p))
	if err != nil {
		return xerrors.Errorf("failed to transact message: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return writeTerm(w, m)
}

func messageParam(p httprouter.Params) int64 {
	m := p.ByName("message")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return mi
}
