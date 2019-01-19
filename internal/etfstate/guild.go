package etfstate

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func (s *Server) handleGuildCreate(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Guild create processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	gc, err := discordetf.DecodeGuildCreate(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	eg := new(errgroup.Group)

	eg.Go(func() error {
		if len(gc.Roles) > 0 {
			return s.setETFs(gc.Id, gc.Roles, s.fmtRoleKey)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Members) > 0 {
			return s.setETFs(gc.Id, gc.Members, s.fmtMemberKey)
		}
		return nil
	})
	eg.Go(func() error {
		if len(gc.Channels) > 0 {
			return s.setETFs(gc.Id, gc.Channels, s.fmtChannelKey)
		}
		return nil
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

func (s *Server) handleGuildDelete(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusOK)
	ctx.SetBodyString("Guild delete processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	gc, err := discordetf.DecodeGuildCreate(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	eg := new(errgroup.Group)

	eg.Go(func() error {
		return s.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(s.fmtRoleKey(gc.Id, 0))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})
	eg.Go(func() error {
		return s.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(s.fmtMemberKey(gc.Id, 0))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})
	eg.Go(func() error {
		return s.Transact(func(t fdb.Transaction) error {
			rg, err := fdb.PrefixRange(s.fmtChannelKey(gc.Id, 0))
			if err != nil {
				return err
			}

			t.ClearRange(rg)
			return nil
		})
	})

	err = eg.Wait()

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished guild_delete",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return err
}
