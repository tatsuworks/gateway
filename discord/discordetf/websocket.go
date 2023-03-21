package discordetf

import (
	"golang.org/x/xerrors"
)

func (_ decoder) DecodeHello(buf []byte) (hbInterval int, trace string, _ error) {
	var (
		d = &etfDecoder{buf: buf}
	)

	err := d.readUntilData()
	if err != nil {
		return 0, "", xerrors.Errorf("read until data: %w", err)
	}

	err = d.checkByte(ettMap)
	if err != nil {
		return 0, "", xerrors.Errorf("verify map byte: %w", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, "", xerrors.Errorf("read map key: %w", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "_trace":
			err := d.checkByte(ettList)
			if err != nil {
				return 0, "", xerrors.Errorf("verify _trace byte: %w", err)
			}

			l := d.readListLen()
			if l != 2 {
				return 0, "", xerrors.Errorf("found more than one _trace value. expected 1 got %d", l-1)
			}

			a, err := d.readAtomWithTag()
			if err != nil {
				return 0, "", xerrors.Errorf("read _trace: %w", err)
			}

			trace = string(d.buf[d.off-a : d.off])
			d.read(1)

		case "heartbeat_interval":
			hbInterval, err = d.readIntWithTagIntoInt()
			if err != nil {
				return 0, "", xerrors.Errorf("read heartbeat_interval: %w", err)
			}
		default:
			return 0, "", xerrors.Errorf("unknown key found in hello event: %w", err)
		}

	}

	return hbInterval, trace, nil
}

func (_ decoder) DecodeReady(buf []byte) (guilds map[int64][]byte, version int, sessionID string,
	resumeGatewayURL string, _ error) {
	d := &etfDecoder{buf: buf}

	err := d.checkByte(ettMap)
	if err != nil {
		return nil, 0, "", "", xerrors.Errorf("verify map byte: %s", err)
	}

	arity := d.readMapLen()
	for ; arity > 0; arity-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return nil, 0, "", "", xerrors.Errorf("read map key: %s", err)
		}

		key := string(d.buf[d.off-l : d.off])
		switch key {
		case "v":
			version, err = d.readIntWithTagIntoInt()
			if err != nil {
				return nil, 0, "", "", xerrors.Errorf("read version: %s", err)
			}

		case "session_id":
			a, err := d.readAtomWithTag()
			if err != nil {
				return nil, 0, "", "", xerrors.Errorf("read session_id: %s", err)
			}

			sessionID = string(d.buf[d.off-a : d.off])

		case "resume_gateway_url":
			a, err := d.readAtomWithTag()
			if err != nil {
				return nil, 0, "", "", xerrors.Errorf("read resume_gateway_url: %s", err)
			}

			resumeGatewayURL = string(d.buf[d.off-a : d.off])

		default:
			err := d.readTerm()
			if err != nil {
				return nil, 0, "", "", xerrors.Errorf("read ready field %s: %w", key, err)
			}
		}

	}

	return nil, version, sessionID, resumeGatewayURL, nil
}
