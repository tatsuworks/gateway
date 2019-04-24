package discordetf

import (
	"fmt"

	"github.com/pkg/errors"
)

const etfStartingByte byte = 131

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

	// verify distribution header
	err := d.checkByte(etfStartingByte)
	if err != nil {
		return e, errors.Wrap(err, "failed to verify etf starting byte")
	}

	//err = D.readTerm()
	//if err != nil {
	//	return T, data, err
	//}

	// verify map byte
	err = d.checkByte(ettMap)
	if err != nil {
		return e, errors.Wrap(err, "failed to verify starting map byte")
	}

	fields := d.readMapLen()

	for ; fields > 0; fields-- {
		// key
		err := d.checkByte(ettAtom)
		if err != nil {
			return e, errors.Wrap(err, "failed to verify map key byte")
		}

		l := d.readRawAtom()
		key := string(d.buf[d.off-l : d.off])

		switch key {
		case "op":
			e.Op, err = d.readSmallIntWithTagIntoInt()
			if err != nil {
				return e, errors.Wrap(err, "failed to read OP value")
			}

		case "d":
			raw, err := d.readTermIntoSlice()
			if err != nil {
				return e, errors.Wrap(err, "failed to read D value")
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

				return e, errors.Wrap(err, "failed to read s value")
			}
			e.S = int64(i)
		case "t":
			l, err := d.readAtomWithTag()
			if err != nil {
				return e, errors.Wrap(err, "failed to read event type")
			}
			e.T = string(d.buf[d.off-l : d.off])

		default:
			return e, errors.Errorf("unknown map key %s", string(key))
		}
	}

	return e, nil
}

func (d *decoder) readIntWithTagIntoInt() (int, error) {
	t := d.read(1)[0]
	switch t {
	case ettSmallInteger:
		return d.readSmallIntIntoInt(), nil
	case ettInteger:
		return d.readRawIntIntoInt(), nil
	default:
		return 0, errors.Errorf("expected bytes 97/98, got %v", t)
	}
}

func (d *decoder) readUntilData() error {
	// verify distribution header
	err := d.checkByte(etfStartingByte)
	if err != nil {
		return errors.Wrap(err, "failed to verify etf starting byte")
	}

	// verify map byte
	err = d.checkByte(ettMap)
	if err != nil {
		return errors.Wrap(err, "failed to verify starting map byte")
	}

	fields := d.readMapLen()

	for ; fields > 0; fields-- {
		// key
		err := d.checkByte(ettAtom)
		if err != nil {
			return errors.Wrap(err, "failed to verify map key byte")
		}

		l := d.readRawAtom()
		key := string(d.buf[d.off-l : d.off])
		if key == "d" {
			return nil
		}

		err = d.readTerm()
		if err != nil {
			return err
		}
	}

	return errors.New("couldn't find data key")
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
	err := d.checkByte(ettAtom)
	if err != nil {
		d.inc(-1)
		if err := d.checkByte(ettBinary); err == nil {
			return d.readRawBinary(), nil
		}

		return 0, err
	}

	return d.readRawAtom(), nil
}

func (d *decoder) readEmojiID() (interface{}, error) {
	var (
		id   int64
		name string
	)
	err := d.checkByte(ettMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to verify emoji map byte")
	}

	arity := d.readMapLen()

	for ; arity > 0; arity-- {
		start := d.off
		l, err := d.readAtomWithTag()
		if err != nil {
			fmt.Println(d.buf)
			fmt.Println(d.buf[start-5:])
			return nil, errors.Wrap(err, "failed to read emoji map key")
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "id":
			id, err = d.readSmallBigWithTagToInt64()
			if err != nil {
				return nil, errors.Wrap(err, "failed to read emoji id")
			}
			continue
		case "name":
			l, err := d.readAtomWithTag()
			if err != nil {
				return nil, errors.Wrap(err, "failed to read emoji name")
			}

			name = string(d.buf[d.off-l : d.off])
			continue
		}

		err = d.readTerm()
		if err != nil {
			return nil, errors.Wrap(err, "failed to read emoji value")
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
		return errors.Wrap(err, "failed to verify map byte")
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return errors.Wrap(err, "failed to read map key")
		}

		key := string(d.buf[d.off-l : d.off])
		if key == name {
			return nil
		}

		err = d.readTerm()
		if err != nil {
			return errors.Wrap(err, "failed to read map value")
		}
	}

	return errors.Errorf("couldn't find key %s", name)
}
