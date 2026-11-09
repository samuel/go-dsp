package ax25

import (
	"bytes"
	"testing"
)

// sender turns frames into the bit stream a modem would hand to Feed: an
// opening flag, the frame and its FCS with a zero stuffed after every five
// consecutive one bits, then a closing flag. Bytes go out least significant bit
// first, which is the order Feed reassembles them in.
type sender struct {
	dec  *Decoder
	ones int
	// tap, when set, sees every bit on its way to the decoder, so a test can
	// compare the stream against a fixture instead of only what it decodes to.
	tap    func(byte)
	frames []*Frame
}

func (s *sender) bit(b byte) {
	if s.tap != nil {
		s.tap(b)
	}
	if f := s.dec.Feed(b); f != nil {
		s.frames = append(s.frames, f)
	}
}

func (s *sender) flag() {
	s.ones = 0
	for _, b := range [8]byte{0, 1, 1, 1, 1, 1, 1, 0} {
		s.bit(b)
	}
}

func (s *sender) bytes(buf []byte) {
	for _, by := range buf {
		for i := range 8 {
			b := (by >> i) & 1
			s.bit(b)
			if b == 0 {
				s.ones = 0
				continue
			}
			if s.ones++; s.ones == 5 {
				s.bit(0)
				s.ones = 0
			}
		}
	}
}

// send frames one payload, appending its FCS, and returns every frame the
// decoder produced.
func (s *sender) send(payload []byte) []*Frame {
	before := len(s.frames)
	fcs := FCS(payload)
	s.flag()
	s.bytes(append(append([]byte{}, payload...), fcs[0], fcs[1]))
	s.flag()
	return s.frames[before:]
}

// encodeAddress builds one seven-byte AX.25 address field.
func encodeAddress(callsign string, ssid int, last, c bool) []byte {
	buf := make([]byte, 7)
	for i := range 6 {
		ch := byte(' ')
		if i < len(callsign) {
			ch = callsign[i]
		}
		buf[i] = ch << 1
	}
	buf[6] = byte(ssid&0xf) << 1
	if last {
		buf[6] |= 1
	}
	if c {
		buf[6] |= 0x80
	}
	return buf
}

// flexNetHeader packs a compressed header: a QSO number, then the destination
// callsign as six 6-bit characters offset from 0x20, most significant first.
func flexNetHeader(qso int, callsign string, ssid int, command bool) []byte {
	h := make([]byte, 7)
	h[0] = byte(qso >> 6)
	h[1] = byte(qso&0x3f)<<2 | 1 // bit 0 marks the compressed header
	if command {
		h[1] |= 2
	}
	var c [6]byte
	for i := range c {
		if i < len(callsign) {
			c[i] = callsign[i] - 0x20
		}
	}
	h[2] = c[0]<<2 | c[1]>>4
	h[3] = c[1]<<4 | c[2]>>2
	h[4] = c[2]<<6 | c[3]
	h[5] = c[4]<<2 | c[5]>>4
	h[6] = c[5]<<4 | byte(ssid)&0xf
	return h
}

// TestFeedUnterminatedAddressField checks that an address field which never
// ends is rejected rather than half-decoded.
//
// The repeater loop stops on either the extension bit or running out of room
// for another seven-byte address. Only the first of those is a whole frame: in
// the second the remaining bytes are the front of a truncated address, and
// reading them as a control field produces a frame that looks entirely
// plausible -- the FCS passed, so these are the bytes that were sent, they just
// are not a complete address field.
func TestFeedUnterminatedAddressField(t *testing.T) {
	// Destination, source and one repeater, none of them with the extension
	// bit, then a seven-byte remainder the loop cannot consume.
	payload := bytes.Join([][]byte{
		encodeAddress("APRS", 0, false, true),
		encodeAddress("N0CALL", 7, false, false),
		encodeAddress("WIDE1", 1, false, false),
		encodeAddress("WIDE2", 2, false, false),
	}, nil)

	s := &sender{dec: NewDecoder()}
	if frames := s.send(payload); len(frames) != 0 {
		t.Fatalf("decoded %d frames from an address field that never ends: %+v", len(frames), frames[0])
	}

	// The same bytes with the extension bit set on the last address are a
	// valid frame, so the rejection is about the bit and not about the length.
	payload = bytes.Join([][]byte{
		encodeAddress("APRS", 0, false, true),
		encodeAddress("N0CALL", 7, false, false),
		encodeAddress("WIDE1", 1, false, false),
		encodeAddress("WIDE2", 2, true, false),
		{0x03, 0xf0},
	}, nil)
	s = &sender{dec: NewDecoder()}
	frames := s.send(payload)
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if n := len(frames[0].Repeaters); n != 2 {
		t.Fatalf("got %d repeaters, want 2: %+v", n, frames[0].Repeaters)
	}
}

func TestFeedNormalHeader(t *testing.T) {
	payload := bytes.Join([][]byte{
		encodeAddress("APRS", 0, false, true),
		encodeAddress("N0CALL", 7, true, false),
		{0x03, 0xf0},
		[]byte("hello world"),
	}, nil)

	s := &sender{dec: NewDecoder()}
	frames := s.send(payload)
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	f := frames[0]
	if f.Destination.Callsign != "APRS" || f.Destination.SSID != 0 {
		t.Errorf("destination = %v", f.Destination)
	}
	if f.Source.Callsign != "N0CALL" || f.Source.SSID != 7 {
		t.Errorf("source = %v", f.Source)
	}
	if len(f.Repeaters) != 0 {
		t.Errorf("repeaters = %v", f.Repeaters)
	}
	if f.Type != UFrame || f.UnnumberedType != UI {
		t.Errorf("type = %v/%v, want U/UI", f.Type, f.UnnumberedType)
	}
	if f.PID != NoLayer3Protocol {
		t.Errorf("PID = %v", f.PID)
	}
	if string(f.Info) != "hello world" {
		t.Errorf("info = %q", f.Info)
	}
	// The C bits differ, so this is a version 2 frame and a command.
	if f.V1 || !f.Command {
		t.Errorf("V1 = %v, Command = %v, want false/true", f.V1, f.Command)
	}
}

func TestFeedRepeaters(t *testing.T) {
	payload := bytes.Join([][]byte{
		encodeAddress("APRS", 0, false, false),
		encodeAddress("N0CALL", 1, false, false),
		encodeAddress("WIDE1", 1, false, false),
		encodeAddress("WIDE2", 2, true, false),
		{0x03, 0xf0},
		[]byte("via"),
	}, nil)

	frames := (&sender{dec: NewDecoder()}).send(payload)
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	f := frames[0]
	want := []Address{{"WIDE1", 1}, {"WIDE2", 2}}
	if len(f.Repeaters) != len(want) {
		t.Fatalf("repeaters = %v, want %v", f.Repeaters, want)
	}
	for i, a := range want {
		if f.Repeaters[i] != a {
			t.Errorf("repeater %d = %v, want %v", i, f.Repeaters[i], a)
		}
	}
	if string(f.Info) != "via" {
		t.Errorf("info = %q", f.Info)
	}
}

func TestFeedControlFields(t *testing.T) {
	header := func() []byte {
		return bytes.Join([][]byte{
			encodeAddress("DEST", 0, false, false),
			encodeAddress("SRC", 0, true, false),
		}, nil)
	}
	for _, tc := range []struct {
		name    string
		control byte
		want    Frame
	}{
		// I frame: bit 0 clear, N(S) in bits 1-3, P in bit 4, N(R) in bits 5-7.
		{"iframe", 0x50, Frame{Type: IFrame, SendSeq: 0, RecvSeq: 2, PollFinal: true}},
		{"iframe-seq", 0x26, Frame{Type: IFrame, SendSeq: 3, RecvSeq: 1}},
		{"sabm", 0x2f | 0x10, Frame{Type: UFrame, UnnumberedType: SABM, PollFinal: true}},
		{"ui", 0x03, Frame{Type: UFrame, UnnumberedType: UI}},
		{"rr", 0x41, Frame{Type: SFrame, SupervisoryType: RR, RecvSeq: 2}},
		{"rej", 0x09, Frame{Type: SFrame, SupervisoryType: REJ}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := append(header(), tc.control, 0xf0, 'x')
			frames := (&sender{dec: NewDecoder()}).send(payload)
			if len(frames) != 1 {
				t.Fatalf("got %d frames, want 1", len(frames))
			}
			f := frames[0]
			if f.Type != tc.want.Type {
				t.Errorf("type = %v, want %v", f.Type, tc.want.Type)
			}
			if f.UnnumberedType != tc.want.UnnumberedType {
				t.Errorf("unnumbered = %v, want %v", f.UnnumberedType, tc.want.UnnumberedType)
			}
			if f.SupervisoryType != tc.want.SupervisoryType {
				t.Errorf("supervisory = %v, want %v", f.SupervisoryType, tc.want.SupervisoryType)
			}
			if f.SendSeq != tc.want.SendSeq || f.RecvSeq != tc.want.RecvSeq {
				t.Errorf("seq = %d/%d, want %d/%d", f.SendSeq, f.RecvSeq, tc.want.SendSeq, tc.want.RecvSeq)
			}
			if f.PollFinal != tc.want.PollFinal {
				t.Errorf("pollFinal = %v, want %v", f.PollFinal, tc.want.PollFinal)
			}
		})
	}
}

// TestFeedFlexNetHeader covers the compressed header. Every 6-bit character
// extraction masked the wrong width or applied its mask to only one side of an
// or, so all but two of the six came out corrupted.
func TestFeedFlexNetHeader(t *testing.T) {
	for _, callsign := range []string{"N0CALL", "APRS", "AB1CDE", "Z"} {
		payload := append(flexNetHeader(1234, callsign, 5, true), 0x03, 0xf0, 'h', 'i')
		frames := (&sender{dec: NewDecoder()}).send(payload)
		if len(frames) != 1 {
			t.Fatalf("%q: got %d frames, want 1", callsign, len(frames))
		}
		f := frames[0]
		if f.Destination.Callsign != callsign {
			t.Errorf("destination = %q, want %q", f.Destination.Callsign, callsign)
		}
		if f.Destination.SSID != 5 {
			t.Errorf("SSID = %d, want 5", f.Destination.SSID)
		}
		if f.QSO != 1234 {
			t.Errorf("QSO = %d, want 1234", f.QSO)
		}
		if f.V1 || !f.Command {
			t.Errorf("V1 = %v, Command = %v, want false/true", f.V1, f.Command)
		}
		if string(f.Info) != "hi" {
			t.Errorf("info = %q", f.Info)
		}
	}
}

// TestFeedInfoNotAliased is the regression test for Info pointing into the
// decoder's receive buffer, which Feed truncates and appends the next frame
// into. A caller that held on to a Frame across one more Feed used to see the
// following frame's bytes.
func TestFeedInfoNotAliased(t *testing.T) {
	header := bytes.Join([][]byte{
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
		{0x03, 0xf0},
	}, nil)

	s := &sender{dec: NewDecoder()}
	first := s.send(append(bytes.Clone(header), []byte("first frame")...))
	if len(first) != 1 {
		t.Fatalf("got %d frames, want 1", len(first))
	}
	held := first[0]
	s.send(append(bytes.Clone(header), []byte("SECOND FRAME")...))
	if string(held.Info) != "first frame" {
		t.Errorf("Info became %q after decoding another frame", held.Info)
	}
}

// TestFeedRejectsBadFCS checks that a corrupted frame is dropped.
func TestFeedRejectsBadFCS(t *testing.T) {
	payload := bytes.Join([][]byte{
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
		{0x03, 0xf0},
		[]byte("payload"),
	}, nil)

	s := &sender{dec: NewDecoder()}
	fcs := FCS(payload)
	s.flag()
	s.bytes(append(append([]byte{}, payload...), fcs[0]^0x01, fcs[1]))
	s.flag()
	if len(s.frames) != 0 {
		t.Errorf("accepted a frame with a bad FCS: %+v", s.frames[0])
	}
}

// TestFeedAbort checks that seven one bits abandon the frame in progress.
func TestFeedAbort(t *testing.T) {
	payload := bytes.Join([][]byte{
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
		{0x03, 0xf0},
		[]byte("payload"),
	}, nil)

	s := &sender{dec: NewDecoder()}
	s.flag()
	s.bytes(payload[:8])
	for range 8 {
		s.bit(1) // abort: more than six consecutive ones
	}
	// A whole valid frame after the abort still has to decode.
	if frames := s.send(payload); len(frames) != 1 {
		t.Fatalf("got %d frames after an abort, want 1", len(frames))
	}
}

func TestFeedTooShort(t *testing.T) {
	s := &sender{dec: NewDecoder()}
	// Under the ten-byte minimum, so processFrame gives up before reading an
	// address field.
	if frames := s.send([]byte{1, 2, 3}); len(frames) != 0 {
		t.Errorf("accepted a %d byte frame", len(frames))
	}
	// Long enough to pass the length check but not to hold two addresses.
	if frames := s.send(bytes.Repeat([]byte{0x40}, 12)); len(frames) != 0 {
		t.Errorf("accepted a truncated normal header: %+v", frames[0])
	}
}

func TestFCSRoundTrip(t *testing.T) {
	for _, frame := range [][]byte{{}, {0x01}, {0x7e, 0x00, 0xff}, []byte("KISS ME"), bytes.Repeat([]byte{0xaa}, 300)} {
		fcs := FCS(frame)
		withFCS := append(append([]byte{}, frame...), fcs[0], fcs[1])
		if !CheckFCS(withFCS) {
			t.Errorf("FCS %x rejected for %x", fcs, frame)
		}
		for i := range withFCS {
			withFCS[i] ^= 0x80
			if CheckFCS(withFCS) {
				t.Errorf("accepted a frame corrupted at byte %d: %x", i, withFCS)
			}
			withFCS[i] ^= 0x80
		}
	}
}

// FuzzFeed feeds arbitrary bytes as a bit stream. Nothing about the input is
// trusted, so the only property is that the decoder neither panics nor hands
// back a frame whose fields contradict each other.
func FuzzFeed(f *testing.F) {
	f.Add([]byte{0x7e, 0x7e, 0x7e})
	f.Add(bytes.Join([][]byte{
		{0x7e},
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
		{0x03, 0xf0, 'x', 0x7e},
	}, nil))
	f.Fuzz(func(t *testing.T, data []byte) {
		ax := NewDecoder()
		for _, by := range data {
			for i := range 8 {
				frame := ax.Feed(by >> i & 1)
				if frame == nil {
					continue
				}
				if frame.Type == IFrame && (frame.SendSeq > 7 || frame.RecvSeq > 7) {
					t.Fatalf("sequence numbers out of range: %+v", frame)
				}
				if len(frame.Destination.Callsign) > 6 || len(frame.Source.Callsign) > 6 {
					t.Fatalf("callsign too long: %+v", frame)
				}
			}
		}
	})
}

// TestStringers covers the String methods, which are how the constant tables
// are read now that they are unexported.
func TestStringers(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{Address{"N0CALL", 7}.String(), "N0CALL-7"},
		{Address{}.String(), "-0"},
		{NoLayer3Protocol.String(), "No Layer 3 Protocol Implemented"},
		{TEXNETDatagramProtocol.String(), "TEXNET datagram protocol"},
		{AppleTalkARP.String(), "AppleTalk ARP"},
		{PID(0x42).String(), "42"},
		{IFrame.String(), "I"},
		{SFrame.String(), "S"},
		{UFrame.String(), "U"},
		{FrameType(9).String(), "09"},
		{SABM.String(), "SABM"},
		{UI.String(), "UI"},
		{UnnumberedType(0x11).String(), "11"},
		{RR.String(), "RR"},
		{RNR.String(), "RNR"},
		{REJ.String(), "REJ"},
		{SREJ.String(), "SREJ"},
		{SupervisoryType(0x7).String(), "07"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

// TestDecoderReset checks that a decoder mid-frame can be handed to a new
// stream.
func TestDecoderReset(t *testing.T) {
	payload := bytes.Join([][]byte{
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
		{0x03, 0xf0},
		[]byte("payload"),
	}, nil)

	s := &sender{dec: NewDecoder()}
	s.flag()
	s.bytes(payload[:6]) // abandoned mid-frame
	s.dec.Reset()
	if frames := s.send(payload); len(frames) != 1 {
		t.Fatalf("got %d frames after Reset, want 1", len(frames))
	}
}

// TestFeedBufferOverflow checks that a frame longer than the receive buffer is
// abandoned rather than allowed to grow without bound.
func TestFeedBufferOverflow(t *testing.T) {
	s := &sender{dec: NewDecoder()}
	s.flag()
	s.bytes(bytes.Repeat([]byte{0x00}, 600)) // past the 512 byte cap
	s.flag()
	if len(s.frames) != 0 {
		t.Errorf("accepted an oversized frame: %+v", s.frames[0])
	}

	// And the decoder still works afterwards.
	payload := bytes.Join([][]byte{
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
		{0x03, 0xf0, 'x'},
	}, nil)
	if frames := s.send(payload); len(frames) != 1 {
		t.Fatalf("got %d frames after an overflow, want 1", len(frames))
	}
}

// TestFeedNoInfo covers the frames that end before the PID and before the
// control field, which are valid and carry no payload.
func TestFeedNoInfo(t *testing.T) {
	addresses := bytes.Join([][]byte{
		encodeAddress("DEST", 0, false, false),
		encodeAddress("SRC", 0, true, false),
	}, nil)

	// Control field, no PID and no info.
	frames := (&sender{dec: NewDecoder()}).send(append(bytes.Clone(addresses), 0x03))
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if frames[0].Info != nil {
		t.Errorf("Info = %q, want nil", frames[0].Info)
	}

	// A FlexNet header with nothing after it.
	frames = (&sender{dec: NewDecoder()}).send(append(flexNetHeader(7, "AB", 0, false), 0, 0, 0))
	if len(frames) != 1 {
		t.Fatalf("got %d FlexNet frames, want 1", len(frames))
	}
}

// handStuffedUIFrame is one AX.25 UI frame's HDLC bit stream, written out here
// rather than produced by sender: an opening flag, then
//
//	82 a0 a4 a6 40 40 60   APRS   dest, SSID 0, address field continues
//	9c 60 86 82 98 98 61   N0CALL source, SSID 0, address field ends
//	03                     UI control field, P/F clear
//	f0                     PID, no layer 3
//	7e ff 41               info
//	be 86                  FCS, low byte first
//
// least significant bit first with a zero after every five consecutive ones,
// then a closing flag. The bits and the FCS were produced by a generator
// written from the format and from CRC-16/X-25's reflected polynomial, sharing
// nothing with this package: sender applies the same stuffing rule the decoder
// undoes, so the two agreeing proves only that they are inverses. The info
// bytes are chosen to force stuffing -- 7e is a flag's own bit pattern and ff
// is eight ones -- and three zeros are stuffed in all, at the three places
// where the run reaches five.
const handStuffedUIFrame = "0111111001000001000001010010010101100101000000100000001000000110" +
	"0011100100000110011000010100000100011001000110011000011011000000" +
	"00001111011111010111110111100000100111110010110000101111110"

// TestFeedHandStuffedFrame decodes the fixture above, so the destuffing rule is
// checked against a bit stream nothing in this package produced.
func TestFeedHandStuffedFrame(t *testing.T) {
	if want := 16 + 8*21 + 3; len(handStuffedUIFrame) != want {
		t.Fatalf("fixture is %d bits, want %d", len(handStuffedUIFrame), want)
	}

	d := NewDecoder()
	var frames []*Frame
	for i, c := range handStuffedUIFrame {
		var bit byte
		switch c {
		case '0':
		case '1':
			bit = 1
		default:
			t.Fatalf("bit %d is %q", i, c)
		}
		if f := d.Feed(bit); f != nil {
			frames = append(frames, f)
		}
	}
	if len(frames) != 1 {
		t.Fatalf("decoded %d frames, want 1", len(frames))
	}
	f := frames[0]
	if got, want := f.Destination.String(), "APRS-0"; got != want {
		t.Errorf("Destination = %s, want %s", got, want)
	}
	if got, want := f.Source.String(), "N0CALL-0"; got != want {
		t.Errorf("Source = %s, want %s", got, want)
	}
	if f.Repeaters != nil {
		t.Errorf("Repeaters = %v, want none", f.Repeaters)
	}
	if !f.V1 {
		t.Error("V1 = false, want true: the two C bits match")
	}
	if f.Command {
		t.Error("Command = true, want false")
	}
	if f.PollFinal {
		t.Error("PollFinal = true, want false")
	}
	if f.Type != UFrame {
		t.Errorf("Type = %v, want %v", f.Type, UFrame)
	}
	if f.UnnumberedType != UI {
		t.Errorf("UnnumberedType = %v, want %v", f.UnnumberedType, UI)
	}
	if f.PID != NoLayer3Protocol {
		t.Errorf("PID = %v, want %v", f.PID, NoLayer3Protocol)
	}
	if want := []byte{0x7e, 0xff, 0x41}; !bytes.Equal(f.Info, want) {
		t.Errorf("Info = %v, want %v", f.Info, want)
	}
}

// TestSenderMatchesHandStuffedFrame ties the fixture to the helper the rest of
// this file uses. sender stuffing the same frame has to produce the same bits,
// which is what makes the other tests' bit streams trustworthy rather than
// merely self-consistent -- on their own they show only that the encoder and
// the decoder are inverses of each other.
func TestSenderMatchesHandStuffedFrame(t *testing.T) {
	var bits []byte
	s := &sender{dec: NewDecoder(), tap: func(b byte) { bits = append(bits, '0'+b) }}
	s.send([]byte{
		0x82, 0xa0, 0xa4, 0xa6, 0x40, 0x40, 0x60,
		0x9c, 0x60, 0x86, 0x82, 0x98, 0x98, 0x61,
		0x03, 0xf0, 0x7e, 0xff, 0x41,
	})
	if string(bits) != handStuffedUIFrame {
		t.Errorf("sender produced\n%s\nwant\n%s", bits, handStuffedUIFrame)
	}
}
