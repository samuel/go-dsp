package ax25

import (
	"fmt"
)

type Address struct {
	Callsign string
	SSID     int // Secondary Station ID
}

func (a Address) String() string {
	return fmt.Sprintf("%s-%d", a.Callsign, a.SSID)
}

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
}

type AX25 struct {
	bitstream     byte
	inFrame       bool
	rxBitI        int
	rxBits        byte
	rxBuf         []byte
	maxBufferSize int
}

func NewDecoder() *AX25 {
	return &AX25{
		rxBuf:         make([]byte, 0, 512),
		maxBufferSize: 512,
	}
}

func parseAddress(buf []byte) Address {
	i := 0
	for ; i < 6; i++ {
		buf[i] >>= 1
		if buf[i] == 0x20 {
			break
		}
	}
	return Address{Callsign: string(buf[:i]), SSID: int((buf[6] >> 1) & 0xf)}
}

func (ax *AX25) processFrame() *Frame {
	if len(ax.rxBuf) < 10 {
		return nil
	}

	if !checkCrcCcitt(ax.rxBuf) {
		return nil
	}

	buf := ax.rxBuf[:len(ax.rxBuf)-2]

	// for i, b := range buf {
	// 	fmt.Printf("%d %08b\n", i, b)
	// }

	frame := Frame{
		V1:      true,
		Command: false, // command (true) or response (false)
	}

	if buf[1]&1 > 0 {
		// FlexNet Header Compression
		frame.V1 = false
		frame.Command = (buf[1] & 2) != 0
		var dest []byte
		if i := (buf[2] >> 2) & 0x2f; i != 0 {
			dest = append(dest, i+0x20)
		}
		if i := (buf[2] << 4) | ((buf[3]>>4)&0xf)&0x3f; i != 0 {
			dest = append(dest, i+0x20)
		}
		if i := (buf[3] << 2) | ((buf[4]>>6)&3)&0x3f; i != 0 {
			dest = append(dest, i+0x20)
		}
		if i := buf[4] & 0x3f; i != 0 {
			dest = append(dest, i+0x20)
		}
		if i := (buf[5] >> 2) & 0x3f; i != 0 {
			dest = append(dest, i+0x20)
		}
		if i := ((buf[5] << 4) | ((buf[6] >> 4) & 0xf)) & 0x3f; i != 0 {
			dest = append(dest, i+0x20)
		}
		if dest != nil {
			frame.Destination = Address{
				Callsign: string(dest),
				SSID:     int(buf[6] & 0xf),
			}
		}
		// TODO
		// fmt.Printf("%s QSO Nr %u", frame.Destination, (buf[0]<<6)|(buf[1]>>2))
		buf = buf[7:]
	} else {
		// Normal Header
		if len(buf) < 15 {
			return nil
		}

		// 6.1.2. Command/Response Procedure
		// dest SSID high bit : buf[6]&0x80 -> C bit of AX.25 frame
		// src SSID high bit : buf[13]&0x80 -> C bit of LA PA frame
		if buf[6]&0x80 != buf[13]&0x80 {
			frame.V1 = false
			frame.Command = int(ax.rxBuf[6]&0x80) != 0
		}

		frame.Destination = parseAddress(buf[:7])
		frame.Source = parseAddress(buf[7:14])

		o := 14
		for ; buf[o-1]&1 == 0 && len(buf)-o > 7; o += 7 {
			frame.Repeaters = append(frame.Repeaters, parseAddress(buf[o:]))
		}
		buf = buf[o:]
	}

	if len(buf) == 0 {
		return &frame
	}

	// 4.2 Control-Field

	controlField := buf[0]
	buf = buf[1:]

	// 4.2.1 & 6.2 Poll/Final bit
	frame.PollFinal = controlField&0x10 != 0

	if controlField&1 == 0 {
		// Info frame
		frame.Type = IFrame
		// 0  : 0
		// 1-3: N(S)
		// 4  : P
		// 5-7: N(R)
		frame.SendSeq = int((controlField >> 1) & 7)
		frame.RecvSeq = int((controlField >> 5) & 7)
	} else if controlField&2 != 0 {
		// Unnumbered frame
		frame.Type = UFrame
		// 4.3.3 Unnumbered Frame Control Fields
		frame.UnnumberedType = UnnumberedType(controlField & ^byte(0x10))
	} else {
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
		frame.Info = buf[1:]
	}
	return &frame
}

// Return a frame when a full one has been received. Otherwise return nil.
func (ax *AX25) Feed(bit int) *Frame {
	ax.bitstream <<= 1
	ax.bitstream |= byte(bit)
	// Watch for flag
	if ax.bitstream&0xff == 0x7e {
		var frame *Frame
		if ax.inFrame && len(ax.rxBuf) > 2 {
			frame = ax.processFrame()
		}
		ax.inFrame = true
		ax.rxBuf = ax.rxBuf[:0]
		ax.rxBits = 0
		ax.rxBitI = 0
		return frame
	}
	// Frame abort
	if ax.bitstream&0x7f == 0x7f {
		ax.inFrame = false
		return nil
	}
	if !ax.inFrame {
		return nil
	}
	// Stuffed bit
	if ax.bitstream&0x3f == 0x3e {
		return nil
	}
	ax.rxBits >>= 1
	if bit != 0 {
		ax.rxBits |= 0x80
	}
	ax.rxBitI++
	if ax.rxBitI == 8 {
		if len(ax.rxBuf) >= ax.maxBufferSize {
			ax.inFrame = false
			// TODO: return an error?
			return nil
		}
		ax.rxBuf = append(ax.rxBuf, ax.rxBits)
		ax.rxBits = 0
		ax.rxBitI = 0
	}
	return nil
}
