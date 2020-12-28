package api

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getChannelMessage(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	channelID, err := channelParam(p)
	if err != nil {
		return xerrors.Errorf("channel param: %w", err)
	}
	messageID, err := messageParam(p)
	if err != nil {
		return xerrors.Errorf("message param: %w", err)
	}
	m, err := s.db.GetChannelMessage(r.Context(), channelID, messageID)
	if err != nil {
		return xerrors.Errorf("transact message: %w", err)
	}

	if m == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, m)
}

func messageParam(p httprouter.Params) (int64, error) {
	m := p.ByName("message")
	mi, err := strconv.ParseInt(m, 10, 64)
	if err != nil {
		return 0, ErrInvalidArgument
	}

	return mi, nil
}
