package etfstate

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/fngdevs/state/discord"
	"github.com/fngdevs/state/etf/discordetf"
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
		return s.setETFs(gc.Id, gc.Roles, s.fmtRoleKey)
	})
	eg.Go(func() error {
		return s.setETFs(gc.Id, gc.Members, s.fmtMemberKey)
	})
	eg.Go(func() error {
		return s.setETFs(gc.Id, gc.Channels, s.fmtChannelKey)
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

func (s *Server) fmtRoleKey(guild, id int64) fdb.Key {
	return s.subs.Roles.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtMemberKey(guild, id int64) fdb.Key {
	return s.subs.Members.Pack(tuple.Tuple{guild, id})
}

func (s *Server) fmtChannelKey(guild, id int64) fdb.Key {
	return s.subs.Channels.Pack(tuple.Tuple{guild, id})
}
