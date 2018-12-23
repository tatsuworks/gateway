package etfstate

import (
	"net/http"

	"git.abal.moe/tatsu/state/internal/mwerr"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
)

// Transact is a helper around (fdb.Database).Transact which accepts a function that doesn't require a return value.
func (s *Server) Transact(fn func(t fdb.Transaction) error) error {
	_, err := s.fdb.Transact(func(t fdb.Transaction) (ret interface{}, err error) {
		return nil, fn(t)
	})

	return errors.Wrap(err, "failed to commit fdb txn")
}

// ReadTransact is a helper around (fdb.Database).ReadTransact which accepts a function that doesn't require a return value.
func (s *Server) ReadTransact(fn func(t fdb.ReadTransaction) error) error {
	_, err := s.fdb.ReadTransact(func(t fdb.ReadTransaction) (ret interface{}, err error) {
		return nil, fn(t)
	})

	return errors.Wrap(err, "failed to commit fdb read txn")
}

func wrapHandler(fn func(ctx *fasthttp.RequestCtx) error) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		err := fn(ctx)
		if err != nil {
			var (
				msg  = err.Error()
				code = http.StatusInternalServerError
			)

			if perr, ok := err.(mwerr.Public); ok {
				msg, code = perr.Public()
			}

			ctx.Error(msg, code)
		}

	}
}
