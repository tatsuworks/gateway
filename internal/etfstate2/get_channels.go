package etfstate2

import (
	"net/http"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var c []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		c = t.Get(s.fmtChannelKey(0, 0)).MustGet()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to transact channel")
	}

	return nil
}

func (s *Server) getChannels(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	return nil
}
