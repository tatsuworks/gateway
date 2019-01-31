package etfstate

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func (s *Server) handleRoleCreate(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Role add processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	rc, err := discordetf.DecodeRole(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtRoleKey(rc.Guild, rc.Id), rc.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished role add/role update",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleRoleDelete(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Role add processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	rc, err := discordetf.DecodeRoleDelete(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Clear(s.fmtRoleKey(rc.Guild, rc.Id))
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished role delete",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
