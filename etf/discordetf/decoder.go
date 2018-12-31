package discordetf

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

type GuildCreateDecoder struct {
	r io.Reader

	buf [256]byte

	guild    []byte
	channels [][]byte
}

func (s *GuildCreateDecoder) readChannelList() error {
	err := s.checkStartingByte(108)
	if err != nil {
		return errors.Wrap(err, "failed to verify list byte")
	}

	// read list len
	_, err = s.r.Read(s.buf[:4])
	if err != nil {
		return errors.Wrap(err, "failed to read list length")
	}

	left := binary.BigEndian.Uint32(s.buf[:4])
	s.channels = make([][]byte, 0, left)

	for ; left > 0; left-- {

	}

	return nil
}

func (s *GuildCreateDecoder) readRawTerm() ([]byte, error) {

	return nil, nil
}

func (s *GuildCreateDecoder)

func (s *GuildCreateDecoder) checkStartingByte(b byte) error {
	_, err := s.r.Read(s.buf[:1])
	if err != nil {
		return err
	}

	if s.buf[0] != b {
		return errors.Errorf("expected starting byte 108, got %v", s.buf[0])
	}

	return nil
}
