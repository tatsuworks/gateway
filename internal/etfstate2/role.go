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

func (s *Server) handleRoleCreate(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	if _, err := io.Copy(buf, r.Body); err != nil {
		return errors.Wrap(err, "failed to copy body")
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
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

	s.bufs.Put(buf)
	return nil
}

func (s *Server) handleRoleDelete(w http.ResponseWriter, r *http.Request) error {
	buf := s.bufs.Get()
	if _, err := io.Copy(buf, r.Body); err != nil {
		return errors.Wrap(err, "failed to copy body")
	}

	termStart := time.Now()
	ev, err := discordetf.DecodeT(buf.B)
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

	s.bufs.Put(buf)
	return nil
}
