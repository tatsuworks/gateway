package discordetf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"golang.org/x/xerrors"
)

// Erlang external term tags.
const (
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

// Different type lens
const (
	mapLenBytes      = 4
	listLenBytes     = 4
	utf8AtomLenBytes = 2
	smallBigLenBytes = 1
	intLenBytes      = 4
	binaryLenBytes   = 4
	stringLenBytes   = 2
	smallIntLenBytes = 1
)

type decoder struct {
	buf []byte
	off int
}

func (d *decoder) read(n int) []byte {
	b := d.buf[d.off : d.off+n]
	d.inc(n)
	return b
}

func (d *decoder) inc(n int) {
	d.off += n
}

func (d *decoder) reset() {
	d.off = 0
}

func (d *decoder) readListLen() int {
	raw := d.read(listLenBytes)
	// add one for nil byte
	return int(binary.BigEndian.Uint32(raw)) + 1
}

func (d *decoder) readMapLen() int {
	raw := d.read(mapLenBytes)
	return int(binary.BigEndian.Uint32(raw))
}

func (d *decoder) fuck() {
	err := d.checkByte(ettMap)
	if err != nil {
		fmt.Println(err)
		return
	}
}

var powers = []int64{
	int64(math.Pow(256, 0)),
	int64(math.Pow(256, 1)),
	int64(math.Pow(256, 2)),
	int64(math.Pow(256, 3)),
	int64(math.Pow(256, 4)),
	int64(math.Pow(256, 5)),
	int64(math.Pow(256, 6)),
	int64(math.Pow(256, 7)),
}

// readSmallBigWithTagToInt64 reads a small big into an int64 and checks the term tag.
func (d *decoder) readSmallBigWithTagToInt64() (int64, error) {
	err := d.checkByte(ettSmallBig)
	if err != nil {
		d.inc(-1)
		if err := d.checkByte(ettAtom); err == nil {
			// nil
			d.readRawAtom()
			return 0, nil
		}

		return 0, xerrors.Errorf("verify small big byte: %w", err)
	}

	return d.readSmallBigIntoInt64(), nil
}

func (d *decoder) readSmallBigIntoInt64() int64 {
	var (
		i    = d.read(2)
		l    = int(i[0])
		sign = int(i[1])
		b    = d.read(l)
	)

	var result int64
	for i := 0; i < len(b); i++ {
		result += int64(b[i]) * powers[i]
	}
	if sign != 0 {
		result = -result
	}
	return result
}

// readMapWithIDIntoSliceFixGuildID reads a map into a slice, extracting the id
// field if one exists.  Additionally, the guild id is injected into the map.
// It may be plausible to assume that a 0 id means one was not found.
func (d *decoder) readMapWithIDIntoSliceFixGuildID(guildID int64) (int64, []byte, error) {
	var (
		start = d.off
		id    int64
		data  []byte
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return 0, nil, xerrors.Errorf("verify map byte: %w", err)
	}

	_left := d.readMapLen()
	left := _left
	for ; left > 0; left-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, nil, xerrors.Errorf("read map key: %w", err)
		}

		// instead of checking the string every time, check the length first
		if l == 2 {
			if string(d.buf[d.off-l:d.off]) == "id" {
				id, err = d.readSmallBigWithTagToInt64()
				if err != nil {
					return 0, nil, xerrors.Errorf("read id: %w", err)
				}

				continue
			}
		}

		if l == 7 {
			if string(d.buf[d.off-l:d.off]) == "user_id" {
				id, err = d.readSmallBigWithTagToInt64()
				if err != nil {
					return 0, nil, xerrors.Errorf("read user_id: %w", err)
				}

				continue
			}
		}

		if l == 4 {
			key := string(d.buf[d.off-l : d.off])
			if key == "user" {
				id, _, err = d.readMapWithIDIntoSlice()
				if err != nil {
					return 0, nil, xerrors.Errorf("read user_id from object: %w", err)
				}
				continue
			}

			// Roles are a special case. They need to be extracted
			// from this key.
			if key == "role" {
				id, data, err = d.readMapWithIDIntoSliceFixGuildID(guildID)
				if err != nil {
					return 0, nil, xerrors.Errorf("read role_id from object: %w", err)
				}
				continue
			}
		}

		err = d.readTerm()
		if err != nil {
			return 0, nil, xerrors.Errorf("read term: %w", err)
		}
	}

	// Role was extracted
	if len(data) != 0 {
		return id, data, nil
	}

	data = d.buf[start:d.off]
	// Copy
	data = append(data[:0:0], data...)
	// Fix map length
	binary.BigEndian.PutUint32(data[1:5], uint32(_left+1))
	// Append guild id at the end
	data = append(data, atomToBytes("guild_id")...)
	data = append(data, snowflakeToBytes(guildID)...)

	return id, data, nil
}

func atomToBytes(atom string) []byte {
	return append([]byte{ettAtom, byte(len(atom) >> 8), byte(len(atom))}, []byte(atom)...)
}

func snowflakeToBytes(id int64) []byte {
	bytes := reverse(new(big.Int).SetInt64(id).Bytes())
	return append([]byte{ettSmallBig, byte(len(bytes)), 0}, bytes...)
}

func reverse(b []byte) []byte {
	size := len(b)
	hsize := size >> 1

	for i := 0; i < hsize; i++ {
		b[i], b[size-i-1] = b[size-i-1], b[i]
	}

	return b
}

// readMapWithIDIntoSlice reads a map into a slice, extracting the id field if
// one exists. It may be plausible to assume that a 0 id means one was not
// found.
func (d *decoder) readMapWithIDIntoSlice() (int64, []byte, error) {
	var (
		start = d.off
		id    int64
		data  []byte
	)

	err := d.checkByte(ettMap)
	if err != nil {
		return 0, nil, xerrors.Errorf("verify map byte: %w", err)
	}

	left := d.readMapLen()
	for ; left > 0; left-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, nil, xerrors.Errorf("read map key: %w", err)
		}

		// instead of checking the string every time, check the length first
		if l == 2 {
			if string(d.buf[d.off-l:d.off]) == "id" {
				id, err = d.readSmallBigWithTagToInt64()
				if err != nil {
					return 0, nil, xerrors.Errorf("read id: %w", err)
				}

				continue
			}
		}

		if l == 7 {
			if string(d.buf[d.off-l:d.off]) == "user_id" {
				id, err = d.readSmallBigWithTagToInt64()
				if err != nil {
					return 0, nil, xerrors.Errorf("read user_id: %w", err)
				}

				continue
			}
		}

		if l == 4 {
			key := string(d.buf[d.off-l : d.off])
			if key == "user" {
				id, _, err = d.readMapWithIDIntoSlice()
				if err != nil {
					return 0, nil, xerrors.Errorf("read user_id from object: %w", err)
				}
				continue
			}
			if key == "role" {
				id, data, err = d.readMapWithIDIntoSlice()
				if err != nil {
					return 0, nil, xerrors.Errorf("read role_id from object: %w", err)
				}
				continue
			}
		}

		err = d.readTerm()
		if err != nil {
			return 0, nil, xerrors.Errorf("read term: %w", err)
		}
	}

	if len(data) != 0 {
		return id, data, nil
	}

	return id, d.buf[start:d.off], nil
}

// guildIDFromMap extracts a guild id from an ETF map.
func (d *decoder) idFromMap(name string) (int64, error) {
	var id int64

	err := d.checkByte(ettMap)
	if err != nil {
		return 0, xerrors.Errorf("verify map byte: %w", err)
	}

	left := d.readMapLen()
	for ; left > 0; left-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, xerrors.Errorf("read map key: %w", err)
		}

		// instead of checking the string every time, check the length first
		if l == len(name) {
			if string(d.buf[d.off-l:d.off]) == name {
				id, err = d.readSmallBigWithTagToInt64()
				if err != nil {
					return 0, xerrors.Errorf("read id: %w", err)
				}

				continue
			}
		}

		err = d.readTerm()
		if err != nil {
			return 0, xerrors.Errorf("read term: %w", err)
		}
	}

	return id, nil
}

// stringFromMap extracts a string at the given key from a map at the current location.
func (d *decoder) stringFromMap(name string) (string, error) {
	var val string

	err := d.checkByte(ettMap)
	if err != nil {
		d.inc(-1)
		d.readAtomWithTag()
		return "", nil
		// return "", xerrors.Errorf("failed to verify map byte: %w", err)
	}

	left := d.readMapLen()
	for ; left > 0; left-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return "", xerrors.Errorf("read id: %w", err)
		}

		// instead of checking the string every time, check the length first
		if l == len(name) {
			if string(d.buf[d.off-l:d.off]) == name {
				ll, err := d.readAtomWithTag()
				if err != nil {
					return "", xerrors.Errorf("read value at specified key: %w", err)
				}

				val = string(d.buf[d.off-ll : d.off])
				continue
			}
		}

		err = d.readTerm()
		if err != nil {
			return "", xerrors.Errorf("read term: %w", err)
		}
	}

	return val, nil
}

// guildIDFromMap extracts a guild id from an ETF map.
// If guild id doesn't exist in the map it will return 0.
func (d *decoder) guildIDFromMap() (int64, error) {
	var id int64

	err := d.checkByte(ettMap)
	if err != nil {
		return 0, xerrors.Errorf("verify map byte: %w", err)
	}

	left := d.readMapLen()
	for ; left > 0; left-- {
		l, err := d.readAtomWithTag()
		if err != nil {
			return 0, xerrors.Errorf("read id: %w", err)
		}

		// instead of checking the string every time, check the length first
		if l == 8 {
			if string(d.buf[d.off-l:d.off]) == "guild_id" {
				id, err = d.readSmallBigWithTagToInt64()
				if err != nil {
					return 0, xerrors.Errorf("read guild_id: %w", err)
				}

				continue
			}
		}

		err = d.readTerm()
		if err != nil {
			return 0, xerrors.Errorf("read term: %w", err)
		}
	}

	return id, nil
}

func (d *decoder) readInteger() (int64, error) {
	s := d.read(1)

	switch s[0] {
	case ettSmallInteger:
		return int64(d.readSmallIntIntoInt()), nil

	case ettInteger:
		return int64(d.readRawIntIntoInt()), nil

	case ettSmallBig:
		return d.readSmallBigIntoInt64(), nil

	default:
		return 0, xerrors.Errorf("unknown int type: %d", int(s[0]))
	}
}

// readListIntoMapByIDFixGuildID turns a list of ETF maps with an `id` key into
// a Go map by that key, inserting the guild id into each ETF map.
func (d *decoder) readListIntoMapByIDFixGuildID(guildID int64) (map[int64][]byte, error) {
	err := d.checkByte(ettList)
	if err != nil {
		d.inc(-1)
		if err := d.checkByte(ettNil); err == nil {
			return nil, nil
		}

		return nil, xerrors.Errorf("verify list byte: %w", err)
	}

	// readListLen will automatically add the nil byte, but we want to
	// handle it manually here.
	left := d.readListLen() - 1
	out := make(map[int64][]byte, left)

	for ; left > 0; left-- {
		id, b, err := d.readMapWithIDIntoSliceFixGuildID(guildID)
		if err != nil {
			return nil, xerrors.Errorf("read list index: %w", err)
		}

		out[id] = b
	}

	err = d.checkByte(ettNil)
	if err != nil {
		return nil, xerrors.Errorf("verify ending nil byte: %w", err)
	}

	return out, nil
}

// readListIntoMapByID turns a list of ETF maps with an `id` key into a Go map
// by that key.
func (d *decoder) readListIntoMapByID() (map[int64][]byte, error) {
	err := d.checkByte(ettList)
	if err != nil {
		d.inc(-1)
		if err := d.checkByte(ettNil); err == nil {
			return nil, nil
		}

		return nil, xerrors.Errorf("verify list byte: %w", err)
	}

	// readListLen will automatically add the nil byte, but we want to handle it manually here.
	left := d.readListLen() - 1
	out := make(map[int64][]byte, left)

	for ; left > 0; left-- {
		id, b, err := d.readMapWithIDIntoSlice()
		if err != nil {
			return out, err
		}

		out[id] = b
	}

	err = d.checkByte(ettNil)
	if err != nil {
		return nil, xerrors.Errorf("verify ending nil byte: %w", err)
	}

	return out, nil
}

// readTermIntoSlice reads the next term into a slice.
func (d *decoder) readTermIntoSlice() ([]byte, error) {
	start := d.off

	err := d.readTerm()
	if err != nil {
		return nil, err
	}

	return d.buf[start:d.off], nil
}

// readTerm will read the next term, advancing the offset, and returning an
// error if a tag isn't supported.
func (d *decoder) readTerm() (err error) {
	t := d.read(1)

	switch t[0] {
	case ettAtom, ettAtomUTF8:
		d.readRawAtom()
	case ettInteger:
		d.readRawInt()
	case ettSmallBig:
		d.readRawSmallBig()
	case ettBinary:
		d.readRawBinary()
	case ettSmallInteger:
		d.readSmallInt()
	case ettMap:
		err = d.readRawMap()
	case ettList:
		d.readRawList()
	case ettNil:
		// we don't need to do anything here since nil is just one byte
		// anyways
	case ettString:
		d.readRawString()
	case ettNewFloat:
		d.readRawNewFloat()
	default:
		err = xerrors.Errorf("unknown type: %v", t)
	}

	if err != nil {
		return xerrors.Errorf("read raw term into buf: %w", err)
	}

	return nil
}

func (d *decoder) readRawNewFloat() {
	d.inc(8)
}

//
// Note: all functions that have `raw` in them generally means they do not read the term tag.
//

// readRawMap advances the offset past the map at the current offset.
func (d *decoder) readRawMap() error {
	fields := d.readMapLen()

	for ; fields > 0; fields-- {
		// key
		err := d.readTerm()
		if err != nil {
			return xerrors.Errorf("read map key: %w", err)
		}

		// value
		err = d.readTerm()
		if err != nil {
			return xerrors.Errorf("read map value: %w", err)
		}
	}

	return nil
}

// readRawList advances the offset past the list at the current offset, returning an error.
func (d *decoder) readRawList() error {
	left := d.readListLen()

	for ; left > 0; left-- {
		err := d.readTerm()
		if err != nil {
			return err
		}
	}

	return nil
}

// readRawAtom advances the offset past the atom at the current offset,
// returning its length.
func (d *decoder) readRawAtom() int {
	lenRaw := d.read(utf8AtomLenBytes)
	atomLen := int(binary.BigEndian.Uint16(lenRaw))
	d.inc(atomLen)
	return atomLen
}

// readRawInt advances the offset past the int (int32) at the current offset.
func (d *decoder) readRawInt() {
	d.inc(intLenBytes)
}

func (d *decoder) readRawIntIntoInt() int {
	return int(binary.BigEndian.Uint32(d.read(4)))
}

// readRawSmallBig advances the offset past the big small at the current
// offset.
func (d *decoder) readRawSmallBig() {
	// add 1 because of sign byte
	bigLen := int(d.read(smallBigLenBytes)[0]) + 1
	d.inc(bigLen)
}

// readRawBinary advances the offset past the binary tag at the current offset.
func (d *decoder) readRawBinary() int {
	binLenRaw := d.read(binaryLenBytes)
	i := int(binary.BigEndian.Uint32(binLenRaw))
	d.inc(i)
	return i
}

// readRawString advances the offset past the string at the current offset.
func (d *decoder) readRawString() {
	strLenRaw := d.read(stringLenBytes)
	d.inc(int(binary.BigEndian.Uint16(strLenRaw)))
}

// readSmallInt advances the offset past the small int (int8) at the current
// offset.
func (d *decoder) readSmallInt() {
	d.inc(smallIntLenBytes)
}

// readRawNil does nothing because the tag byte is it's entire length.
func (d *decoder) readRawNil() {}

// checkByte checks the byte at the current offset and returns an error if it
// is not equal to expected.
func (d *decoder) checkByte(expected byte) error {
	b := d.read(1)

	if b[0] != expected {
		return xerrors.Errorf("expected byte %v, got byte %v", expected, b[0])
	}

	return nil
}
