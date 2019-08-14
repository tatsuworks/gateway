package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

var ErrNotFound = xerrors.New("resource not found")

func wrapHandler(fn func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		err := fn(w, r, p)
		if err != nil {
			var (
				msg  = err.Error()
				code = http.StatusInternalServerError
			)

			if xerrors.Is(err, ErrNotFound) {
				code = http.StatusNotFound
			}

			http.Error(w, msg, code)
		}
	})
}
