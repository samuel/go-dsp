package ax25

import (
	"errors"
	"fmt"
)

// Encoder turns frames into the HDLC bit stream a Decoder takes, one bit per
// byte: opening flags, the frame and its FCS least significant bit first with a
// zero stuffed after every five consecutive ones, then a closing flag. NRZI
// turns the bits into the levels a modulator sends.
type Encoder struct {
	// Preamble is how many flags open each frame. A receiver needs a run of
	// them to lock its bit clock before the frame starts; zero sends one.
	Preamble int
}

// AppendFrame appends the bits of f to dst.
//
// The header is always the normal one: the destination, the source and up to
// eight repeaters, each a callsign of up to six characters and an SSID of 0 to
// 15. V1 sets both C bits clear; otherwise Command sets the destination's C bit
// and Command false the source's. PID and Info are sent on an I frame and on a
// UI frame and must be empty on any other. A FlexNet compressed header is not
// produced, so QSO is ignored.
func (e Encoder) AppendFrame(dst []byte, f *Frame) ([]byte, error) {
	payload, err := f.AppendBinary(nil)
	if err != nil {
		return dst, err
	}
	return e.AppendPayload(dst, payload), nil
}

// AppendPayload appends the bits of a frame given as the bytes between its
// flags, without the FCS, which it computes and sends.
func (e Encoder) AppendPayload(dst, payload []byte) []byte {
	for range max(e.Preamble, 1) {
		dst = appendFlag(dst)
	}
	fcs := FCS(payload)
	ones := 0
	dst = appendStuffed(dst, &ones, payload)
	dst = appendStuffed(dst, &ones, fcs[:])
	return appendFlag(dst)
}

// flagBits is 0x7e least significant bit first.
var flagBits = [8]byte{0, 1, 1, 1, 1, 1, 1, 0}

func appendFlag(dst []byte) []byte { return append(dst, flagBits[:]...) }

// appendStuffed appends buf least significant bit first, with a zero after
// every five consecutive ones. ones carries the run across calls.
func appendStuffed(dst []byte, ones *int, buf []byte) []byte {
	for _, by := range buf {
		for i := range 8 {
			b := (by >> i) & 1
			dst = append(dst, b)
			if b == 0 {
				*ones = 0
				continue
			}
			if *ones++; *ones == 5 {
				dst = append(dst, 0)
				*ones = 0
			}
		}
	}
	return dst
}

// maxRepeaters is the most digipeaters an AX.25 address field holds.
const maxRepeaters = 8

// AppendBinary appends f as the bytes between its flags, without the FCS: the
// address field, the control field and, on an I or UI frame, the PID and Info.
// It is the inverse of what a Decoder parses, under the rules AppendFrame
// gives.
func (f *Frame) AppendBinary(b []byte) ([]byte, error) {
	if len(f.Repeaters) > maxRepeaters {
		return b, fmt.Errorf("ax25: %d repeaters, more than the %d an address field holds", len(f.Repeaters), maxRepeaters)
	}
	destC, srcC := false, false
	if !f.V1 {
		destC, srcC = f.Command, !f.Command
	}
	var err error
	if b, err = appendAddress(b, f.Destination, false, destC); err != nil {
		return b, err
	}
	if b, err = appendAddress(b, f.Source, len(f.Repeaters) == 0, srcC); err != nil {
		return b, err
	}
	for i, r := range f.Repeaters {
		if b, err = appendAddress(b, r, i == len(f.Repeaters)-1, false); err != nil {
			return b, err
		}
	}

	var pf byte
	if f.PollFinal {
		pf = 0x10
	}
	carriesInfo := false
	switch f.Type {
	case IFrame:
		if f.SendSeq < 0 || f.SendSeq > 7 || f.RecvSeq < 0 || f.RecvSeq > 7 {
			return b, fmt.Errorf("ax25: sequence numbers %d and %d are outside 0 to 7", f.SendSeq, f.RecvSeq)
		}
		b = append(b, byte(f.RecvSeq)<<5|pf|byte(f.SendSeq)<<1)
		carriesInfo = true
	case SFrame:
		if f.RecvSeq < 0 || f.RecvSeq > 7 {
			return b, fmt.Errorf("ax25: sequence number %d is outside 0 to 7", f.RecvSeq)
		}
		if f.SupervisoryType&^0x0c != 0x01 {
			return b, fmt.Errorf("ax25: %v is not a supervisory type", f.SupervisoryType)
		}
		b = append(b, byte(f.RecvSeq)<<5|pf|byte(f.SupervisoryType))
	case UFrame:
		u := byte(f.UnnumberedType)
		if u&0x03 != 0x03 || u&0x10 != 0 {
			return b, fmt.Errorf("ax25: %v is not an unnumbered type", f.UnnumberedType)
		}
		b = append(b, u|pf)
		carriesInfo = f.UnnumberedType == UI
	default:
		return b, fmt.Errorf("ax25: frame type %v", f.Type)
	}

	if !carriesInfo {
		if f.PID != 0 || len(f.Info) != 0 {
			return b, fmt.Errorf("ax25: a %v frame carries no PID or Info", f.Type)
		}
		return b, nil
	}
	b = append(b, byte(f.PID))
	return append(b, f.Info...), nil
}

// appendAddress appends one seven-byte address field: the callsign shifted up
// a bit and space padded, then the SSID byte with the reserved bits set, the C
// bit and the extension bit that ends the address field.
func appendAddress(b []byte, a Address, last, c bool) ([]byte, error) {
	if len(a.Callsign) > 6 {
		return b, fmt.Errorf("ax25: callsign %q is longer than six characters", a.Callsign)
	}
	if a.SSID < 0 || a.SSID > 15 {
		return b, fmt.Errorf("ax25: SSID %d is outside 0 to 15", a.SSID)
	}
	for i := range 6 {
		ch := byte(' ')
		if i < len(a.Callsign) {
			ch = a.Callsign[i]
			if ch <= ' ' || ch > '~' {
				return b, errors.New("ax25: a callsign holds printable ASCII without spaces")
			}
		}
		b = append(b, ch<<1)
	}
	ssid := 0x60 | byte(a.SSID)<<1
	if c {
		ssid |= 0x80
	}
	if last {
		ssid |= 1
	}
	return append(b, ssid), nil
}

// NRZI is the line code AX.25 runs over: a 0 bit is a change of level and a 1
// bit is none, so a long run of ones, which bit stuffing caps, is the only
// thing that holds the level still. It converts in one direction per value,
// carrying the last level across calls; the level starts at 0.
type NRZI struct {
	level byte
}

// Encode appends the level each bit of bits leaves the line at, 0 or 1.
func (z *NRZI) Encode(dst, bits []byte) []byte {
	for _, b := range bits {
		if b&1 == 0 {
			z.level ^= 1
		}
		dst = append(dst, z.level)
	}
	return dst
}

// Decode appends the bit each level of levels carries: 1 where it equals the
// level before it and 0 where it changed. A nonzero level counts as 1.
func (z *NRZI) Decode(dst, levels []byte) []byte {
	for _, l := range levels {
		if l != 0 {
			l = 1
		}
		var b byte
		if l == z.level {
			b = 1
		}
		z.level = l
		dst = append(dst, b)
	}
	return dst
}

// Reset returns the level to 0.
func (z *NRZI) Reset() { z.level = 0 }
