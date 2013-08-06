package borip

import (
	"encoding/binary"
	"errors"
	"net"
)

const (
	defaultBufferSize = 256 * 1024
	packetHeaderSize  = 4
)

var ErrShortPacket = errors.New("borip: short packet")

const (
	FlagNone            = 0x00
	FlagHardwareOverrun = 0x01 // Used at hardware interface
	FlagNetworkOverrun  = 0x02 // Used at client (network too slow)
	FlagBufferOverrun   = 0x04 // Used at client (client consumer too slow)
	FlagEmptyPayload    = 0x08 // Reserved
	FlagStreamStart     = 0x10 // Used for first packet of newly started stream
	FlagStremEnd        = 0x20 // Reserved (TO DO: Server sends BF_EMPTY_PAYLOAD | BF_STREAM_END)
	FlagBufferUnderrun  = 0x40 // Used at hardware interface
	FlagHardwareTimeout = 0x80 // Used at hardware interface
)

type PacketHeader struct {
	Flags        byte
	Notification byte   // Reserved (currently 0)
	Idx          uint16 // Sequence number (incremented each time a packet is sent, used by client to count dropped packets)
}

type PacketReader struct {
	conn        net.PacketConn
	buf         []byte
	bufI, bufN  int
	withHeaders bool
	header      PacketHeader
}

func NewPacketReader(conn net.PacketConn, withHeaders bool) *PacketReader {
	return &PacketReader{
		conn:        conn,
		buf:         make([]byte, defaultBufferSize),
		withHeaders: withHeaders,
	}
}

func (rd *PacketReader) Header() PacketHeader {
	return rd.header
}

func (rd *PacketReader) ReadSamples(samples []complex128) (int, error) {
	if rd.bufI >= rd.bufN {
		n, _, err := rd.conn.ReadFrom(rd.buf)
		if err != nil {
			return 0, err
		}
		rd.bufI = 0
		rd.bufN = n
		if rd.withHeaders {
			if n < packetHeaderSize {
				return 0, ErrShortPacket
			}
			rd.header.Flags = rd.buf[rd.bufI]
			rd.header.Notification = rd.buf[rd.bufI+1]
			rd.header.Idx = binary.LittleEndian.Uint16(rd.buf[2:4])
			rd.bufI += 4
		}
		// rd.bufN = n - (n & 7)
	}
	idx := 0
	for rd.bufI < rd.bufN {
		i_real := int16(binary.LittleEndian.Uint16(rd.buf[rd.bufI : rd.bufI+2]))
		q_imag := int16(binary.LittleEndian.Uint16(rd.buf[rd.bufI+2 : rd.bufI+4]))
		samples[idx] = complex(float64(i_real), float64(q_imag))
		idx++
		rd.bufI += 4
	}
	return idx, nil
}
