package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"cdr.dev/slog"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

var ErrNotFound = sql.ErrNoRows
var ErrInvalidArgument = errors.New("invalid argument provided")

func wrapHandler(log slog.Logger, fn func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		start := time.Now()
		err := fn(w, r, p)
		if err != nil {
			var (
				msg  = err.Error()
				code = http.StatusInternalServerError
			)

			if xerrors.Is(err, ErrNotFound) {
				code = http.StatusNotFound
			}

			if xerrors.Is(err, ErrInvalidArgument) {
				code = http.StatusBadRequest
			}

			log := log.With(
				slog.F("query", r.URL.Query().Encode()),
				slog.F("method", r.Method),
				slog.F("path", r.URL.Path),
				slog.F("took", time.Since(start)),
				slog.F("status_code", code),
			)

			logLevelFn := log.Debug
			if code >= 500 {
				logLevelFn = log.Error
			}

			logLevelFn(r.Context(), msg, slog.Error(err))
			http.Error(w, msg, code)
		}
	})
}
