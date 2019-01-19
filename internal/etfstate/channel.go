package etfstate

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func (s *Server) handleChannelCreate(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Channel create processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	ch, err := discordetf.DecodeChannel(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtChannelKey(ch.Guild, ch.Id), ch.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished channel_create",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleChannelDelete(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusOK)
	ctx.SetBodyString("Channel delete processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	ch, err := discordetf.DecodeChannel(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Clear(s.fmtChannelKey(ch.Guild, ch.Id))
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished channel_delete",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
