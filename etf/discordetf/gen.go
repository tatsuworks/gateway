package discordetf

import (
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
	Op uint8
	S  uint8
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
			e.Op, err = d.readSmallIntWithIndicatorIntoInt()
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
			i, err := d.readSmallIntWithIndicatorIntoInt()
			if err != nil {
				return e, errors.Wrap(err, "failed to read s value")
			}
			e.S = i
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

func (d *decoder) readSmallIntWithIndicatorIntoInt() (uint8, error) {
	err := d.checkByte(ettSmallInteger)
	if err != nil {
		return 0, err
	}

	d.inc(1)
	return uint8(d.buf[d.off-1]), nil
}

func (d *decoder) readAtomWithTag() (int, error) {
	err := d.checkByte(ettAtom)
	if err != nil {
		return 0, err
	}

	return d.readRawAtom(), nil
}
