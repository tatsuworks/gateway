package etfstate2

import (
	"io"
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/fngdevs/state/etf/discordetf"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (s *Server) handleMessageCreate(w http.ResponseWriter, r *http.Request) error {
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

	mc, err := discordetf.DecodeMessage(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtMessageKey(mc.Channel, mc.Id), mc.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished message create",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleMessageDelete(w http.ResponseWriter, r *http.Request) error {
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

	mc, err := discordetf.DecodeMessage(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Clear(s.fmtMessageKey(mc.Channel, mc.Id))
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished message update",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleMessageReactionAdd(w http.ResponseWriter, r *http.Request) error {
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

	rc, err := discordetf.DecodeMessageReaction(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtMessageReactionKey(rc.Channel, rc.Message, rc.User, rc.Name), rc.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished message reaction add",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}

func (s *Server) handleMessageReactionRemove(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	if _, err := io.Copy(buf, r.Body); err != nil {
		return errors.Wrap(err, "failed to copy body")
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
	if err != nil {
		return err
	}

	rc, err := discordetf.DecodeMessageReaction(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Clear(s.fmtMessageReactionKey(rc.Channel, rc.Message, rc.User, rc.Name))
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished message reaction remove",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	s.bufs.Put(buf)
	return nil
}

func (s *Server) handleMessageReactionRemoveAll(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	if _, err := io.Copy(buf, r.Body); err != nil {
		return errors.Wrap(err, "failed to copy body")
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
	if err != nil {
		return err
	}

	rc, err := discordetf.DecodeMessageReactionRemoveAll(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		pre, err := fdb.PrefixRange(s.fmtMessageReactionKey(rc.Channel, rc.Message, rc.User, ""))
		if err != nil {
			return errors.Wrap(err, "failed to make message reaction prefixrange")
		}

		t.ClearRange(pre)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished message reaction remove all",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
