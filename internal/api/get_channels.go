package api

import (
	"encoding/binary"
	"io"
	"net/http"
	"strconv"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/julienschmidt/httprouter"
	"github.com/pkg/errors"
)

func (s *Server) getChannel(w http.ResponseWriter, r *http.Request, p httprouter.Params) error {
	var c []byte

	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		c = t.Get(s.fmtChannelKey(guildParam(p), channelParam(p))).MustGet()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to transact channel")
	}

	if c == nil {
		return errors.New("channel not found")
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
	var raws []fdb.KeyValue

	pre, _ := fdb.PrefixRange(s.fmtChannelsKey(guildParam(p)))
	err := s.ReadTransact(func(t fdb.ReadTransaction) error {
		raws = t.Snapshot().GetRange(pre, FDBRangeWantAll).GetSliceOrPanic()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "failed to read channels")
	}

	return writeTerms(w, raws)
}

// Erlang external term tags.
const (
	ettStart         = byte(131)
	ettAtom          = byte(100)
	ettAtomUTF8      = byte(118) // this is beyond retarded
	ettBinary        = byte(109)
	ettBitBinary     = byte(77)
	ettCachedAtom    = byte(67)
	ettCacheRef      = byte(82)
	ettExport        = byte(113)
	ettFloat         = byte(99)
	ettFun           = byte(117)
	ettInteger       = byte(98)
	ettLargeBig      = byte(111)
	ettLargeTuple    = byte(105)
	ettList          = byte(108)
	ettNewCache      = byte(78)
	ettNewFloat      = byte(70)
	ettNewFun        = byte(112)
	ettNewRef        = byte(114)
	ettNil           = byte(106)
	ettPid           = byte(103)
	ettPort          = byte(102)
	ettRef           = byte(101)
	ettSmallAtom     = byte(115)
	ettSmallAtomUTF8 = byte(119) // this is beyond retarded
	ettSmallBig      = byte(110)
	ettSmallInteger  = byte(97)
	ettSmallTuple    = byte(104)
	ettString        = byte(107)
	ettMap           = byte(116)
)

func writeTerms(w io.Writer, raws []fdb.KeyValue) error {
	if err := writeSliceHeader(w, len(raws)); err != nil {
		return err
	}

	for _, e := range raws {
		_, err := w.Write(e.Value)
		if err != nil {
			return errors.Wrap(err, "failed to write term")
		}
	}

	_, err := w.Write([]byte{ettNil})
	return errors.Wrap(err, "failed to write ending nil")
}

func writeTerm(w io.Writer, raw []byte) error {
	if err := writeETFHeader(w); err != nil {
		return err
	}

	_, err := w.Write(raw)
	return errors.Wrap(err, "failed to write term")
}

func writeSliceHeader(w io.Writer, len int) error {
	var h [6]byte
	h[0], h[1] = ettStart, ettList
	binary.BigEndian.PutUint32(h[2:], uint32(len))

	_, err := w.Write(h[:])
	return errors.Wrap(err, "failed to write slice header")
}

func writeETFHeader(w io.Writer) error {
	_, err := w.Write([]byte{131})
	return errors.Wrap(err, "failed to write etf starting byte")
}
