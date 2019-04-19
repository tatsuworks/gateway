package discordetf

import "github.com/pkg/errors"

func DecodeHello(buf []byte) (int, string, error) {
	var (
		d        = &decoder{buf: buf}
		trace    string
		interval int
	)

	err := d.readUntilData()
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to read until data")
	}

	err = d.checkByte(ettMap)
	if err != nil {
		return 0, "", errors.Wrap(err, "failed to verify map byte")
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, "", err
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "_trace":
			err := d.checkByte(ettList)
			if err != nil {
				return 0, "", errors.Wrap(err, "failed to verify _trace list byte")
			}

			l := d.readListLen()
			if l != 2 {
				return 0, "", errors.Errorf("found more than one _trace: %d", l)
			}

			a, err := d.readAtomWithTag()
			if err != nil {
				return 0, "", errors.Wrap(err, "failed to read _trace list item")
			}

			trace = string(d.buf[d.off-a : d.off])
			d.read(1)

		case "heartbeat_interval":
			interval, err = d.readIntWithTagIntoInt()
			if err != nil {
				return 0, "", errors.Wrap(err, "failed to read heartbeat_interval")
			}
		default:
			return 0, "", errors.Errorf("unknown key found in hello event: %s", key)
		}

	}

	return interval, trace, nil
}
