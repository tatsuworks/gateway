package discordetf

import (
	"unsafe"

	"golang.org/x/xerrors"

	"github.com/tatsuworks/gateway/discord"
)

const etfStartingByte = byte(131)

var (
	dRaw  = []byte("d")
	opRaw = []byte("op")
	sRaw  = []byte("s")
	tRaw  = []byte("t")
)

func (_ decoder) DecodeT(buf []byte) (*discord.Event, error) {
	var (
		d = &etfDecoder{buf: buf}
		e = &discord.Event{}
	)

	err := d.checkByte(etfStartingByte)
	if err != nil {
		return e, xerrors.Errorf("verify etf starting byte: %w", err)
	}

	err = d.checkByte(ettMap)
	if err != nil {
		return e, xerrors.Errorf("verify starting map byte: %w", err)
	}

	fields := d.readMapLen()
	for ; fields > 0; fields-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return e, xerrors.Errorf("map key: %w", err)
		}

		_key := d.buf[d.off-l : d.off]
		key := *(*string)(unsafe.Pointer(&_key))

		switch key {
		case "op":
			e.Op, err = d.readSmallIntWithTagIntoInt()
			if err != nil {
				return e, xerrors.Errorf("read op value: %w", err)
			}

		case "d":
			raw, err := d.readTermIntoSlice()
			if err != nil {
				return e, xerrors.Errorf("read d value: %w", err)
			}
			e.D = raw

		case "s":
			i, err := d.readIntWithTagIntoInt()
			if err != nil {
				d.inc(-1)
				_, err2 := d.readAtomWithTag()
				if err2 == nil {
					continue
				}

				return e, xerrors.Errorf("read s value: %w", err)
			}
			e.S = int64(i)
		case "t":
			l, err := d.readAtomWithTag()
			if err != nil {
				return e, xerrors.Errorf("read event type: %w", err)
			}

			_t := d.buf[d.off-l : d.off]
			e.T = *(*string)(unsafe.Pointer(&_t))

		default:
			return e, xerrors.Errorf("unknown map key %s", string(key))
		}
	}

	return e, nil
}

func (d *etfDecoder) iterateMap(fn func(key string) error) error {
	return nil
}

func (d *etfDecoder) readIntWithTagIntoInt() (int, error) {
	t := d.read(1)[0]
	switch t {
	case ettSmallInteger:
		return d.readSmallIntIntoInt(), nil
	case ettInteger:
		return d.readRawIntIntoInt(), nil
	default:
		return 0, xerrors.Errorf("expected bytes %d/%d, got %v", ettSmallInteger, ettInteger, t)
	}
}

func (d *etfDecoder) readUntilData() error {
	err := d.checkByte(etfStartingByte)
	if err != nil {
		return xerrors.Errorf("verify starting etf byte: %w", err)
	}

	err = d.checkByte(ettMap)
	if err != nil {
		return xerrors.Errorf("verify starting map byte: %w", err)
	}

	fields := d.readMapLen()
	for ; fields > 0; fields-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return xerrors.Errorf("read map keyleveledrolesbyscore: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		if key == "d" {
			return nil
		}

		err = d.readTerm()
		if err != nil {
			return err
		}
	}

	return xerrors.New("couldn't find data key")
}

func (d *etfDecoder) readSmallIntWithTagIntoInt() (int, error) {
	err := d.checkByte(ettSmallInteger)
	if err != nil {
		return 0, err
	}

	d.inc(1)
	return int(d.buf[d.off-1]), nil
}

func (d *etfDecoder) readSmallIntIntoInt() int {
	return int(d.read(1)[0])
}

func (d *etfDecoder) readAtomWithTag() (int, error) {
	t := d.read(1)[0]
	switch t {
	case ettAtom:
		return d.readRawAtom(), nil
	case ettBinary:
		return d.readRawBinary(), nil
	default:
		return 0, xerrors.Errorf("expected bytes %d/%d, got %v", ettAtom, ettBinary, t)
	}
}

func (d *etfDecoder) readEmojiID() (interface{}, error) {
	var (
		id   int64
		name string
	)
	err := d.checkByte(ettMap)
	if err != nil {
		return nil, xerrors.Errorf("verify map byte: %w", err)
	}

	arity := d.readMapLen()

	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return nil, xerrors.Errorf("read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "id":
			id, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return nil, xerrors.Errorf("read id: %w", err)
			}
			continue
		case "name":
			l, err := d.readAtomWithTag()
			if err != nil {
				return nil, xerrors.Errorf("read name: %w", err)
			}

			name = string(d.buf[d.off-l : d.off])
			continue
		}

		err = d.readTerm()
		if err != nil {
			return nil, xerrors.Errorf("read value: %w", err)
		}
	}

	if id == 0 {
		return name, err
	}

	return id, err
}

func (d *etfDecoder) readUntilKey(name string) error {
	err := d.checkByte(ettMap)
	if err != nil {
		return xerrors.Errorf("verify map byte: %w", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return xerrors.Errorf("read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		if key == name {
			return nil
		}

		err = d.readTerm()
		if err != nil {
			return xerrors.Errorf("read map value: %w", err)
		}
	}

	return xerrors.Errorf("couldn't find key %s", name)
}
