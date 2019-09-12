package api

import (
	"encoding/binary"
	"io"
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	c, err := s.db.GetChannel(channelParam(p))
	if err != nil {
		return xerrors.Errorf("failed to read channel: %w", err)
	}

	if c == nil {
		return ErrNotFound
	}

	return writeTerm(w, c)
}

func channelParam(p httprouter.Params) int64 {
	c := p.ByName("channel")
	ci, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		panic(err.Error())
	}

	return ci
}

func (s *Server) getChannels(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	cs, err := s.db.GetChannels()
	if err != nil {
		return xerrors.Errorf("failed to read channels: %w", err)
	}

	return writeTerms(w, cs)
}

func (s *Server) getGuildChannels(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	cs, err := s.db.GetGuildChannels(guildParam(p))
	if err != nil {
		return xerrors.Errorf("failed to read guild channels: %w", err)
	}

	return writeTerms(w, cs)
}

// Erlang external term tags.
const (
	ettStart = byte(131)
	ettList  = byte(108)
	ettNil   = byte(106)
)

func writeTerms(w io.Writer, raws []fdb.KeyValue) error {
	if err := writeSliceHeader(w, len(raws)); err != nil {
		return err
	}

	for _, e := range raws {
		_, err := w.Write(e.Value)
		if err != nil {
			return xerrors.Errorf("failed to write term: %w", err)
		}
	}

	_, err := w.Write([]byte{ettNil})
	if err != nil {
		return xerrors.Errorf("failed to write ending nil: %w", err)
	}

	return nil
}

func writeTermsRaw(w io.Writer, raws [][]byte) error {
	if err := writeSliceHeader(w, len(raws)); err != nil {
		return err
	}

	for _, e := range raws {
		_, err := w.Write(e)
		if err != nil {
			return xerrors.Errorf("failed to write term: %w", err)
		}
	}

	_, err := w.Write([]byte{ettNil})
	if err != nil {
		return xerrors.Errorf("failed to write ending nil: %w", err)
	}

	return nil
}

func writeTerm(w io.Writer, raw []byte) error {
	if err := writeETFHeader(w); err != nil {
		return err
	}

	_, err := w.Write(raw)
	return err
}

func writeSliceHeader(w io.Writer, len int) error {
	var h [6]byte
	h[0], h[1] = ettStart, ettList
	binary.BigEndian.PutUint32(h[2:], uint32(len))

	_, err := w.Write(h[:])
	if err != nil {
		return xerrors.Errorf("failed to write slice header: %w", err)
	}

	return nil
}

func writeETFHeader(w io.Writer) error {
	_, err := w.Write([]byte{131})
	if err != nil {
		return xerrors.Errorf("failed to write etf starting byte: %w", err)
	}

	return nil
}
