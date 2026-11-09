// Package ax25 decodes AX.25 packet radio frames from an HDLC bit stream, the
// link layer amateur packet and APRS run over.
//
// A Decoder is fed one received bit at a time and hands back a Frame each time a
// complete, correctly checksummed one arrives. FCS builds the frame check
// sequence a transmitter needs, over the same table.
package ax25

import (
	"bytes"
	"fmt"
)

// Address is one AX.25 station address: a callsign of up to six characters and
// a secondary station ID distinguishing that operator's stations.
type Address struct {
	Callsign string
	SSID     int // Secondary Station ID
}

// String formats the address as CALLSIGN-SSID. An address a compressed FlexNet
// header did not carry has an empty callsign and formats as "-0".
func (a Address) String() string {
	return fmt.Sprintf("%s-%d", a.Callsign, a.SSID)
}

// Frame is one decoded AX.25 frame. Which fields are meaningful depends on
// Type: SendSeq only on an I frame, SupervisoryType only on an S frame,
// UnnumberedType only on a U frame, and PID and Info only on an I frame or an
// unnumbered information frame.
type Frame struct {
	Source           Address
	Destination      Address
	Repeaters        []Address
	V1               bool
	Command          bool // command=true, response=false
	Type             FrameType
	SendSeq, RecvSeq int
	PollFinal        bool // P/F of 1 is true, 0 is false
	UnnumberedType   UnnumberedType
	SupervisoryType  SupervisoryType
	PID              PID // Protocol Identifier
	Info             []byte
	// QSO is the FlexNet QSO number. It identifies the link a
	// compressed-header frame belongs to, and is only set for one of those --
	// which is also the only case where Source is left unset, because a
	// compressed header does not carry it.
	QSO int
}

// Decoder reassembles frames from a stream of received bits. It is not safe for
// concurrent use, and it keeps state between calls, so one Decoder follows one
// stream.
type Decoder struct {
	bitstream     byte
	inFrame       bool
	rxBitI        int
	rxBits        byte
	rxBuf         []byte
	maxBufferSize int
}

// NewDecoder returns a Decoder waiting for the first flag of a frame.
func NewDecoder() *Decoder {
	return &Decoder{
		rxBuf:         make([]byte, 0, 512),
		maxBufferSize: 512,
	}
}

// Reset abandons any frame in progress, so the decoder can be reused on a new
// stream.
func (d *Decoder) Reset() {
	d.bitstream = 0
	d.inFrame = false
	d.rxBitI = 0
	d.rxBits = 0
	d.rxBuf = d.rxBuf[:0]
}

// parseAddress decodes one seven-byte AX.25 address field: six callsign
// characters shifted up by one bit, space padded, then an SSID byte.
//
// The characters are unpacked into a local rather than shifted down in place,
// since the caller's C-bit tests still need the untouched bytes.
func parseAddress(buf []byte) Address {
	var callsign [6]byte
	n := 0
	for ; n < len(callsign); n++ {
		c := buf[n] >> 1
		if c == ' ' {
			break
		}
		callsign[n] = c
	}
	return Address{Callsign: string(callsign[:n]), SSID: int((buf[6] >> 1) & 0xf)}
}

// processFrame decodes the bytes accumulated between two flags, or returns nil
// if they do not form a valid frame.
func (d *Decoder) processFrame() *Frame {
	if len(d.rxBuf) < 10 {
		return nil
	}

	if !CheckFCS(d.rxBuf) {
		return nil
	}

	buf := d.rxBuf[:len(d.rxBuf)-2]

	frame := Frame{V1: true}

	if buf[1]&1 > 0 {
		// FlexNet header compression. The two ends of the link are identified
		// by a QSO number rather than by a pair of callsigns, so only the
		// destination is carried and Source stays unset.
		frame.V1 = false
		frame.Command = (buf[1] & 2) != 0
		frame.QSO = int(buf[0])<<6 | int(buf[1]>>2)

		// Six 6-bit characters packed most significant first across buf[2:7],
		// each offset from 0x20, with the low nibble of buf[6] holding the
		// SSID. Mask to six bits after combining the shifts: & binds tighter
		// than |, so a trailing mask would cover the right operand alone.
		var dest []byte
		for _, c := range [6]byte{
			buf[2] >> 2,
			(buf[2]<<4 | buf[3]>>4) & 0x3f,
			(buf[3]<<2 | buf[4]>>6) & 0x3f,
			buf[4] & 0x3f,
			buf[5] >> 2,
			(buf[5]<<4 | buf[6]>>4) & 0x3f,
		} {
			if c != 0 {
				dest = append(dest, c+0x20)
			}
		}
		if dest != nil {
			frame.Destination = Address{
				Callsign: string(dest),
				SSID:     int(buf[6] & 0xf),
			}
		}
		buf = buf[7:]
	} else {
		// Normal Header
		if len(buf) < 15 {
			return nil
		}

		// 6.1.2 Command/Response: the dest SSID high bit (buf[6]&0x80) carries
		// the C bit in AX.25, the src's (buf[13]&0x80) in LA PA.
		if buf[6]&0x80 != buf[13]&0x80 {
			frame.V1 = false
			frame.Command = buf[6]&0x80 != 0
		}

		frame.Destination = parseAddress(buf[:7])
		frame.Source = parseAddress(buf[7:14])

		o := 14
		for ; buf[o-1]&1 == 0 && len(buf)-o > 7; o += 7 {
			frame.Repeaters = append(frame.Repeaters, parseAddress(buf[o:]))
		}
		if buf[o-1]&1 == 0 {
			// The loop ran out of room before finding the extension bit that
			// ends the address field, so what is left is the tail of a truncated
			// address. Without this a 7-byte remainder is read as a control
			// field and the frame decodes as plausible: the FCS passed, so the
			// bytes are not noise, just not a whole frame.
			return nil
		}
		buf = buf[o:]
	}

	// A frame with an address field and nothing after it. The length checks
	// above happen to rule this out, but only as a consequence of their
	// constants, so the guard stays.
	if len(buf) == 0 {
		return &frame
	}

	// 4.2 Control-Field

	controlField := buf[0]
	buf = buf[1:]

	// 4.2.1 & 6.2 Poll/Final bit
	frame.PollFinal = controlField&0x10 != 0

	switch {
	case controlField&1 == 0:
		// Info frame
		frame.Type = IFrame
		// 0  : 0
		// 1-3: N(S)
		// 4  : P
		// 5-7: N(R)
		frame.SendSeq = int((controlField >> 1) & 7)
		frame.RecvSeq = int((controlField >> 5) & 7)
	case controlField&2 != 0:
		// Unnumbered frame
		frame.Type = UFrame
		// 4.3.3 Unnumbered Frame Control Fields
		frame.UnnumberedType = UnnumberedType(controlField & ^byte(0x10))
	default:
		// Supervisory frame
		frame.Type = SFrame
		frame.SupervisoryType = SupervisoryType(controlField & 0x0f)
		frame.RecvSeq = int((controlField >> 5) & 7)
	}
	if len(buf) == 0 {
		return &frame
	}

	if frame.Type == IFrame || (frame.Type == UFrame && frame.UnnumberedType == UI) {
		frame.PID = PID(buf[0])
		// A copy: buf is a window onto d.rxBuf, which Feed appends the next
		// frame into.
		frame.Info = bytes.Clone(buf[1:])
	}
	return &frame
}

// Feed shifts one received bit into the decoder and returns a frame once a
// complete one has arrived, or nil.
func (d *Decoder) Feed(bit byte) *Frame {
	bit &= 1
	d.bitstream <<= 1
	d.bitstream |= bit
	// Watch for flag
	if d.bitstream&0xff == 0x7e {
		var frame *Frame
		if d.inFrame && len(d.rxBuf) > 2 {
			frame = d.processFrame()
		}
		d.inFrame = true
		d.rxBuf = d.rxBuf[:0]
		d.rxBits = 0
		d.rxBitI = 0
		return frame
	}
	// Frame abort
	if d.bitstream&0x7f == 0x7f {
		d.inFrame = false
		return nil
	}
	if !d.inFrame {
		return nil
	}
	// Stuffed bit
	if d.bitstream&0x3f == 0x3e {
		return nil
	}
	d.rxBits >>= 1
	if bit != 0 {
		d.rxBits |= 0x80
	}
	d.rxBitI++
	if d.rxBitI == 8 {
		if len(d.rxBuf) >= d.maxBufferSize {
			// A frame longer than the limit is noise that happened to open
			// with a flag, so the decoder leaves the frame rather than
			// reporting anything: Feed returns a frame or nothing, and there
			// is no stream to abort.
			d.inFrame = false
			return nil
		}
		d.rxBuf = append(d.rxBuf, d.rxBits)
		d.rxBits = 0
		d.rxBitI = 0
	}
	return nil
}
