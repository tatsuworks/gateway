package discordetf

import (
	"golang.org/x/xerrors"
)

const etfStartingByte = byte(131)

var (
	dRaw  = []byte("d")
	opRaw = []byte("op")
	sRaw  = []byte("s")
	tRaw  = []byte("t")
)

type Event struct {
	D  []byte
	Op int
	S  int64
	T  string
}

func DecodeT(buf []byte) (*Event, error) {
	var (
		d = &decoder{buf: buf}
		e = &Event{}
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

		key := string(d.buf[d.off-l : d.off])

		switch key {
		case "op":
			e.Op, err = d.readSmallIntWithTagIntoInt()
			if err != nil {
				return e, xerrors.Errorf("read OP value: %w", err)
			}

		case "d":
			raw, err := d.readTermIntoSlice()
			if err != nil {
				return e, xerrors.Errorf("read D value: %w", err)
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
			e.T = string(d.buf[d.off-l : d.off])

		default:
			return e, xerrors.Errorf("unknown map key %s", string(key))
		}
	}

	return e, nil
}

func (d *decoder) iterateMap(fn func(key string) error) error {
	return nil
}

func (d *decoder) readIntWithTagIntoInt() (int, error) {
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

func (d *decoder) readUntilData() error {
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

func (d *decoder) readSmallIntWithTagIntoInt() (int, error) {
	err := d.checkByte(ettSmallInteger)
	if err != nil {
		return 0, err
	}

	d.inc(1)
	return int(d.buf[d.off-1]), nil
}

func (d *decoder) readSmallIntIntoInt() int {
	return int(d.read(1)[0])
}

func (d *decoder) readAtomWithTag() (int, error) {
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

func (d *decoder) readEmojiID() (interface{}, error) {
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

func (d *decoder) readUntilKey(name string) error {
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
