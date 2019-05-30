package api

import (
	"io"
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/tatsuworks/state/etf/discordetf"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

func (s *Server) handleChannelCreate(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	_, err := io.Copy(buf, r.Body)
	if err != nil {
		return err
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
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

func (s *Server) handleChannelDelete(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	_, err := io.Copy(buf, r.Body)
	if err != nil {
		return err
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
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

func (s *Server) handleVoiceStateUpdate(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	buf := s.bufs.Get()
	defer func() {
		s.bufs.Put(buf)
		err := r.Body.Close()
		if err != nil {
			s.log.Error("failed to close request body", zap.Error(err))
		}
	}()

	_, err := io.Copy(buf, r.Body)
	if err != nil {
		return err
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
	if err != nil {
		return err
	}

	vs, err := discordetf.DecodeVoiceState(ev.D)
	if err != nil {
		return err
	}

	termStop := time.Since(termStart)
	fdbStart := time.Now()

	err = s.Transact(func(t fdb.Transaction) error {
		t.Set(s.fmtVoiceStateKey(vs.Channel, vs.User), vs.Raw)
		return nil
	})
	if err != nil {
		return err
	}

	fdbStop := time.Since(fdbStart)
	s.log.Info(
		"finished voice_state_update",
		zap.Duration("decode", termStop),
		zap.Duration("fdb", fdbStop),
		zap.Duration("total", termStop+fdbStop),
	)

	return nil
}
