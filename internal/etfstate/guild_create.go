package etfstate

import (
	"bytes"
	"net/http"
	"time"

	"git.abal.moe/tatsu/state/discord"
	"git.abal.moe/tatsu/state/etf"
	"git.abal.moe/tatsu/state/internal/mwerr"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type defaultVals struct {
	Op int
	S  int
	T  string
}

type guildCreate struct {
	D discord.Guild
	defaultVals
}

func (s *Server) guildCreate(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Guild create processed.")

	var (
		buf = bytes.NewBuffer(ctx.Request.Body())
		dec = new(etf.Context).NewDecoder(buf)
	)

	termStart := time.Now()

	term, err := dec.NextTerm()
	if err != nil {
		return &mwerr.EtfErr{E: err}
	}

	gc := new(guildCreate)
	err = etf.TermIntoStruct(term, gc)
	if err != nil {
		return &mwerr.EtfErr{E: err}
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	eg := new(errgroup.Group)

	eg.Go(func() error {
		return s.setETFs(gc.D.Roles, s.fmtRoleKey)
	})
	//eg.Go(func() error {
	//	return s.setETFs(gc.D.Members, s.fmtMemberKey)
	//})
	eg.Go(func() error {
		return s.setETFs(gc.D.Channels, s.fmtChannelKey)
	})

	err = eg.Wait()

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished guild_create",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return err
}

func (s *Server) setETFs(etfs []etf.Map, key func(term etf.Term) fdb.Key) error {
	return s.Transact(func(t fdb.Transaction) error {
		for _, e := range etfs {
			var (
				buf    = s.getBuf()
				etfctx = new(etf.Context)
				err    = etfctx.Write(buf, e)
			)
			if err != nil {
				return errors.Wrap(err, "failed to encode term")
			}

			t.Set(key(e[etf.Atom("id")]), buf.Bytes())
		}
		return nil
	})
}

func (s *Server) fmtRoleKey(term etf.Term) fdb.Key {
	return s.subs.Roles.Pack(tuple.Tuple{term})
}

func (s *Server) fmtMemberKey(term etf.Term) fdb.Key {
	return s.subs.Members.Pack(tuple.Tuple{term})
}

func (s *Server) fmtChannelKey(term etf.Term) fdb.Key {
	return s.subs.Channels.Pack(tuple.Tuple{term})
}
