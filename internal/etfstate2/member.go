package etfstate2

import (
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"go.uber.org/zap"
)

func (s *Server) handleMemberChunk(w http.ResponseWriter, r *http.Request) error {
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

func (s *Server) handleMemberAdd(w http.ResponseWriter, r *http.Request) error {
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

func (s *Server) handleMemberRemove(w http.ResponseWriter, r *http.Request) error {
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

func (s *Server) handlePresenceUpdate(w http.ResponseWriter, r *http.Request) error {
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

	p, err := discordetf.DecodePresence(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtPresenceKey(p.Guild, p.Id), p.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	_ = termStop
	_ = fdbStop
	s.log.Info(
		"finished presence_update",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
