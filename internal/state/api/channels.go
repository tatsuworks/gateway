package api

import (
	"encoding/binary"
	"io"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/xerrors"
)

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	c, err := s.db.GetChannel(r.Context(), channelParam(p))
	if err != nil {
		return xerrors.Errorf("read channel: %w", err)
	}

	if c == nil {
		return ErrNotFound
	}

	return s.writeTerm(w, c)
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
	cs, err := s.db.GetChannels(r.Context())
	if err != nil {
		return xerrors.Errorf("read channels: %w", err)
	}

	return s.writeTerms(w, cs)
}

func (s *Server) getGuildChannels(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	guild, err := guildParam(p)
	if err != nil {
		return xerrors.Errorf("read guild param: %w", err)
	}
	cs, err := s.db.GetGuildChannels(r.Context(), guild)
	if err != nil {
		return xerrors.Errorf("read guild channels: %w", err)
	}

	return s.writeTerms(w, cs)
}

// Erlang external term tags.
const (
	ettStart = byte(131)
	ettList  = byte(108)
	ettNil   = byte(106)
)

func (s *Server) writeTerms(w io.Writer, raws [][]byte) error {
	if s.enc == "etf" {
		if err := writeSliceHeader(w, len(raws)); err != nil {
			return err
		}
	} else if s.enc == "json" {
		w.Write([]byte("["))
	}

	first := true
	writeComma := func() {
		if first {
			first = false
			return
		}

		w.Write([]byte(","))
	}

	for _, e := range raws {
		writeComma()
		_, err := w.Write(e)
		if err != nil {
			return xerrors.Errorf("write term: %w", err)
		}
	}

	if s.enc == "etf" {
		_, err := w.Write([]byte{ettNil})
		if err != nil {
			return xerrors.Errorf("failed to write ending nil: %w", err)
		}
	} else if s.enc == "json" {
		w.Write([]byte("]"))
	}

	return nil
}

func (s *Server) writeTerm(w io.Writer, raw []byte) error {
	if s.enc == "etf" {
		if err := writeETFHeader(w); err != nil {
			return err
		}
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
		return xerrors.Errorf("write slice header: %w", err)
	}

	return nil
}

func writeETFHeader(w io.Writer) error {
	_, err := w.Write([]byte{131})
	if err != nil {
		return xerrors.Errorf("write etf starting byte: %w", err)
	}

	return nil
}
