package etfstate2

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func (s *Server) handleGuildCreate(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
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

func (s *Server) handleGuildDelete(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
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

func (s *Server) handleGuildBanAdd(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
	if err != nil {
		return err
	}

	gb, err := discordetf.DecodeGuildBan(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtGuildBanKey(gb.Guild, gb.User), gb.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished guild_ban_create",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleGuildBanRemove(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
	if err != nil {
		return err
	}

	gb, err := discordetf.DecodeGuildBan(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Clear(s.fmtGuildBanKey(gb.Guild, gb.User))
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished guild_ban_remove",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
