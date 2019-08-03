package discordetf

import (
	"golang.org/x/xerrors"
)

func DecodeHello(buf []byte) (int, string, error) {
	var (
		d        = &decoder{buf: buf}
		trace    string
		interval int
	)

	err := d.readUntilData()
	if err != nil {
		return 0, "", xerrors.Errorf("failed to read until data: %w", err)
	}

	err = d.checkByte(ettMap)
	if err != nil {
		return 0, "", xerrors.Errorf("failed to verify map byte: %w", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, "", xerrors.Errorf("failed to read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "_trace":
			err := d.checkByte(ettList)
			if err != nil {
				return 0, "", xerrors.Errorf("failed to verify _trace byte: %w", err)
			}

			l := d.readListLen()
			if l != 2 {
				return 0, "", xerrors.Errorf("found more than one _trace value. expected 1 got %d", l-1)
			}

			a, err := d.readAtomWithTag()
			if err != nil {
				return 0, "", xerrors.Errorf("failed to read _trace: %w", err)
			}

			trace = string(d.buf[d.off-a : d.off])
			d.read(1)

		case "heartbeat_interval":
			interval, err = d.readIntWithTagIntoInt()
			if err != nil {
				return 0, "", xerrors.Errorf("failed to read heartbeat_interval: %w", err)
			}
		default:
			return 0, "", xerrors.Errorf("unknown key found in hello event: %w", err)
		}

	}

	return interval, trace, nil
}

func DecodeReady(buf []byte) (int, string, error) {
	var (
		d      = &decoder{buf: buf}
		v      int
		sessID string
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return 0, "", xerrors.Errorf("failed to verify map byte: %s", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, "", xerrors.Errorf("failed to read map key: %s", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "v":
			v, err = d.readIntWithTagIntoInt()
			if err != nil {
				return 0, "", xerrors.Errorf("failed to read version: %s", err)
			}

		case "session_id":
			a, err := d.readAtomWithTag()
			if err != nil {
				return 0, "", xerrors.Errorf("failed to read session_id: %s", err)
			}

			sessID = string(d.buf[d.off-a : d.off])

		default:
			err := d.readTerm()
			if err != nil {
				return 0, "", xerrors.Errorf("failed to read ready field %s: %w", key, err)
			}
		}

	}

	return v, sessID, nil
}
