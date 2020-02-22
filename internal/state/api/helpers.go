package api

import (
	"bufio"
	"database/sql"
	"net"
	"net/http"
	"os"
	"time"
	"unsafe"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

var ErrNotFound = sql.ErrNoRows
var log = sloghuman.Make(os.Stdout)

func wrapHandler(fn func(w http.ResponseWriter, r *http.Request, p httprouter.Params) error) httprouter.Handle {
	return LogMW(log)(httprouter.Handle(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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
	}))
}

// StatusWriter intercepts the status of the request.
// It is guaranteed to be the ResponseWriter directly downstream from Middleware.
type StatusWriter struct {
	http.ResponseWriter
	ErrorResponseBody []byte
	Status            int
}

func (w *StatusWriter) WriteHeader(status int) {
	w.Status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusWriter) Write(b []byte) (int, error) {
	if w.Status == 0 {
		w.Status = 200
	}

	if w.Status >= 400 {
		w.ErrorResponseBody = b
	}

	return w.ResponseWriter.Write(b)
}

func (w *StatusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func LogMW(log slog.Logger) func(next httprouter.Handle) httprouter.Handle {
	return func(next httprouter.Handle) httprouter.Handle {
		return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			start := time.Now()
			sw := &StatusWriter{ResponseWriter: w}
			next(sw, r, p)

			log := log.With(
				slog.F("path", r.URL.Path),
				slog.F("took", time.Since(start)),
				slog.F("status_code", sw.Status),
			)

			if sw.Status >= 400 {
				body := *(*string)(unsafe.Pointer(&sw.ErrorResponseBody))
				log = log.With(
					slog.F("response_body", body),
				)
			}

			logLevelFn := log.Info
			if sw.Status >= 500 {
				logLevelFn = log.Error
			}

			logLevelFn(r.Context(), r.Method)
		})
	}
}
