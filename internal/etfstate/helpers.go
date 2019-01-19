package etfstate

import (
	"net/http"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/fngdevs/state/internal/mwerr"
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

func (s *Server) setETFs(guild int64, etfs map[int64][]byte, key func(guild, id int64) fdb.Key) error {
	return s.Transact(func(t fdb.Transaction) error {
		//eg := new(errgroup.Group)

		for id, e := range etfs {
			//eg.Go(func() error {
			//	t.Set(key(guild, id), e)
			//	return nil
			//})
			t.Set(key(guild, id), e)
		}

		//return eg.Wait()
		return nil
	})
}

func (s *Server) fmtGuildKey(guild int64) fdb.Key {
	return s.subs.Guilds.Pack(tuple.Tuple{guild})
}

func (s *Server) fmtRoleKey(guild, id int64) fdb.Key {
	return s.subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtMemberKey(guild, id int64) fdb.Key {
	//s.subs.Members.
	return s.subs.Members.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtChannelKey(guild, id int64) fdb.Key {
	return s.subs.Channels.Pack(tuple.Tuple{guild, id})
}
