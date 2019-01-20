package etfstate

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

func (s *Server) handleMemberChunk(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Member chunk processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	mc, err := discordetf.DecodeMemberChunk(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.setETFs(mc.Guild, mc.Members, s.fmtChannelKey)
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished member_chunk",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleMemberAdd(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetBodyString("Member add processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	mc, err := discordetf.DecodeMember(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtMemberKey(mc.Guild, mc.Id), mc.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished member_add/member_update",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleMemberRemove(ctx *fasthttp.RequestCtx) error {
	// Will be overwritten if error.
	ctx.SetStatusCode(http.StatusOK)
	ctx.SetBodyString("Member remove processed.")

	termStart := time.Now()
	ev, err := discordetf.DecodeT(ctx.Request.Body())
	if err != nil {
		return err
	}

	mc, err := discordetf.DecodeMember(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Clear(s.fmtMemberKey(mc.Guild, mc.Id))
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished member_remove",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
