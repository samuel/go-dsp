package ax25

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// TestEncoderRoundTrip encodes a frame of each kind and checks the Decoder
// hands back the same Frame.
func TestEncoderRoundTrip(t *testing.T) {
	dest := Address{"APRS", 0}
	src := Address{"N0CALL", 7}
	for _, f := range []Frame{
		{Destination: dest, Source: src, V1: true, Type: UFrame, UnnumberedType: UI, PID: NoLayer3Protocol, Info: []byte("hello, world")},
		{Destination: dest, Source: src, Repeaters: []Address{{"WIDE1", 1}, {"WIDE2", 2}}, V1: true,
			Type: UFrame, UnnumberedType: UI, PID: NoLayer3Protocol, Info: []byte("!4903.50N/07201.75W-")},
		{Destination: dest, Source: src, Command: true, Type: IFrame, SendSeq: 3, RecvSeq: 5, PollFinal: true,
			PID: NoLayer3Protocol, Info: bytes.Repeat([]byte{0x7e, 0xff}, 40)},
		{Destination: dest, Source: src, Command: false, Type: SFrame, SupervisoryType: REJ, RecvSeq: 6},
		{Destination: dest, Source: src, Command: true, Type: UFrame, UnnumberedType: SABM, PollFinal: true},
		{Destination: dest, Source: src, Command: false, Type: UFrame, UnnumberedType: UA},
	} {
		bits, err := Encoder{Preamble: 4}.AppendFrame(nil, &f)
		if err != nil {
			t.Fatalf("%+v: %v", f, err)
		}
		d := NewDecoder()
		var got []*Frame
		for _, b := range bits {
			if fr := d.Feed(b); fr != nil {
				got = append(got, fr)
			}
		}
		if len(got) != 1 {
			t.Fatalf("%v %v: decoded %d frames, want 1", f.Type, f.UnnumberedType, len(got))
		}
		if !reflect.DeepEqual(*got[0], f) {
			t.Errorf("decoded\n%+v\nwant\n%+v", *got[0], f)
		}
	}
}

// TestEncoderPreamble checks the opening flags and that a run of them still
// leaves the frame decodable.
func TestEncoderPreamble(t *testing.T) {
	payload := []byte{1, 2, 3}
	one := Encoder{}.AppendPayload(nil, payload)
	five := Encoder{Preamble: 5}.AppendPayload(nil, payload)
	if len(five)-len(one) != 4*8 {
		t.Errorf("a preamble of 5 adds %d bits, want 32", len(five)-len(one))
	}
	if string(bitString(five[:40])) != strings.Repeat("01111110", 5) {
		t.Errorf("preamble %s", bitString(five[:40]))
	}
}

func bitString(bits []byte) []byte {
	s := make([]byte, len(bits))
	for i, b := range bits {
		s[i] = '0' + b
	}
	return s
}

func TestFrameAppendBinaryRejects(t *testing.T) {
	ok := Frame{Destination: Address{"APRS", 0}, Source: Address{"N0CALL", 0}, V1: true, Type: UFrame, UnnumberedType: UI}
	for name, mutate := range map[string]func(*Frame){
		"long callsign":       func(f *Frame) { f.Source.Callsign = "N0CALLX" },
		"SSID 16":             func(f *Frame) { f.Destination.SSID = 16 },
		"space in a callsign": func(f *Frame) { f.Source.Callsign = "N0 C" },
		"nine repeaters":      func(f *Frame) { f.Repeaters = make([]Address, 9) },
		"send sequence 8":     func(f *Frame) { f.Type, f.SendSeq = IFrame, 8 },
		"info on an S frame":  func(f *Frame) { f.Type, f.SupervisoryType, f.Info = SFrame, RR, []byte("x") },
		"bad S type":          func(f *Frame) { f.Type, f.SupervisoryType = SFrame, 0x02 },
		"bad U type":          func(f *Frame) { f.UnnumberedType = 0x13 },
		"PID on a UA":         func(f *Frame) { f.UnnumberedType, f.PID = UA, NoLayer3Protocol },
		"unknown type":        func(f *Frame) { f.Type = 7 },
	} {
		f := ok
		mutate(&f)
		if _, err := f.AppendBinary(nil); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ok.AppendBinary(nil); err != nil {
		t.Errorf("the unmutated frame: %v", err)
	}
}

// TestNRZI checks both directions against a worked sequence and that they are
// inverses across any split.
func TestNRZI(t *testing.T) {
	bits := []byte{1, 0, 0, 1, 1, 0, 1, 0}
	// From level 0: a 0 toggles, a 1 holds.
	want := []byte{0, 1, 0, 0, 0, 1, 1, 0}
	var enc NRZI
	if got := enc.Encode(nil, bits); !bytes.Equal(got, want) {
		t.Fatalf("Encode = %v, want %v", got, want)
	}
	var dec NRZI
	if got := dec.Decode(nil, want); !bytes.Equal(got, bits) {
		t.Fatalf("Decode = %v, want %v", got, bits)
	}

	long := Encoder{Preamble: 3}.AppendPayload(nil, []byte("NRZI round trip"))
	for split := range long {
		var e, d NRZI
		levels := e.Encode(e.Encode(nil, long[:split]), long[split:])
		back := d.Decode(nil, levels[:split/2])
		back = d.Decode(back, levels[split/2:])
		if !bytes.Equal(back, long) {
			t.Fatalf("split %d: round trip differs", split)
		}
	}

	enc.Reset()
	if got := enc.Encode(nil, []byte{1}); got[0] != 0 {
		t.Error("Reset did not return the level to 0")
	}
	if got := (&NRZI{}).Decode(nil, []byte{0, 7, 7}); !bytes.Equal(got, []byte{1, 0, 1}) {
		t.Errorf("a nonzero level is 1: got %v", got)
	}
}
