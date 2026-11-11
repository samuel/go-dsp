package pcm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"samuelks.com/dsp"
)

// TestCodecCoversEveryCell runs every Format against every sample type in
// both directions, so a format or a constraint member added without a kernel
// fails here rather than decoding nothing.
func TestCodecCoversEveryCell(t *testing.T) {
	for _, f := range Formats() {
		t.Run(f.String(), func(t *testing.T) {
			coverCell[float32](t, f)
			coverCell[float64](t, f)
			coverCell[complex64](t, f)
			coverCell[complex128](t, f)
		})
	}
}

func coverCell[T dsp.Sample](t *testing.T, f Format) {
	t.Helper()
	size := f.Width() * elems[T]()
	src := make([]byte, 5*size)
	for i := range src {
		src[i] = byte(i*37 + 11)
	}
	dst := make([]T, 5)
	if n := Decode(dst, src, f); n != 5 {
		t.Fatalf("Decode %T: %d samples, want 5", dst, n)
	}
	out := make([]byte, len(src))
	if n := Encode(out, dst, f); n != 5 {
		t.Fatalf("Encode %T: %d samples, want 5", dst, n)
	}
	// A sample round-trips through any T that holds it exactly: every format
	// but I32 and F64 at float32 width.
	if f.Width() < 4 || (f.Width() == 4 && f.Float()) || isF64[T]() {
		if !bytes.Equal(out, src) && !hasNaN(dst) {
			t.Errorf("Encode(Decode(b)) %T = % x, want % x", dst, out, src)
		}
	}
}

func elems[T dsp.Sample]() int {
	var z T
	switch any(z).(type) {
	case complex64, complex128:
		return 2
	}
	return 1
}

func isF64[T dsp.Sample]() bool {
	var z T
	switch any(z).(type) {
	case float64, complex128:
		return true
	}
	return false
}

func hasNaN[T dsp.Sample](s []T) bool {
	for _, v := range s {
		if v != v { //nolint:gocritic // NaN check
			return true
		}
	}
	return false
}

func TestDecodeScale(t *testing.T) {
	for _, tc := range []struct {
		f    Format
		src  []byte
		want []float64
	}{
		{U8, []byte{0, 64, 128, 192, 255}, []float64{-1, -0.5, 0, 0.5, 127.0 / 128}},
		{I8, []byte{0x80, 0xc0, 0, 0x40, 0x7f}, []float64{-1, -0.5, 0, 0.5, 127.0 / 128}},
		{I16LE, []byte{0x00, 0x80, 0xff, 0x7f, 0x01, 0x00}, []float64{-1, 32767.0 / 32768, 1.0 / 32768}},
		{I16BE, []byte{0x80, 0x00, 0x7f, 0xff, 0x00, 0x01}, []float64{-1, 32767.0 / 32768, 1.0 / 32768}},
		{I24LE, []byte{0x00, 0x00, 0x80, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}, []float64{-1, (1<<23 - 1) / float64(1<<23), -1.0 / (1 << 23)}},
		{I24BE, []byte{0x80, 0x00, 0x00, 0x7f, 0xff, 0xff}, []float64{-1, (1<<23 - 1) / float64(1<<23)}},
		{I32LE, []byte{0, 0, 0, 0x80, 0xff, 0xff, 0xff, 0x7f}, []float64{-1, (1<<31 - 1) / float64(1<<31)}},
		{I32BE, []byte{0x80, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff}, []float64{-1, (1<<31 - 1) / float64(1<<31)}},
		{F32LE, binary.LittleEndian.AppendUint32(nil, math.Float32bits(1.5)), []float64{1.5}},
		{F32BE, binary.BigEndian.AppendUint32(nil, math.Float32bits(-2.25)), []float64{-2.25}},
		{F64LE, binary.LittleEndian.AppendUint64(nil, math.Float64bits(3.0000000001)), []float64{3.0000000001}},
		{F64BE, binary.BigEndian.AppendUint64(nil, math.Float64bits(-7)), []float64{-7}},
	} {
		got := make([]float64, len(tc.want))
		if n := Decode(got, tc.src, tc.f); n != len(tc.want) || !slices.Equal(got, tc.want) {
			t.Errorf("%v: Decode = %d %v, want %v", tc.f, n, got, tc.want)
		}
		got32 := make([]float32, len(tc.want))
		Decode(got32, tc.src, tc.f)
		for i, w := range tc.want {
			if got32[i] != float32(w) {
				t.Errorf("%v: Decode float32 [%d] = %v, want %v", tc.f, i, got32[i], float32(w))
			}
		}
	}
}

// TestDecodeI32Float32TopCode is the one place Decode can reach +1: the top
// I32 code is not representable in a float32 and rounds up.
func TestDecodeI32Float32TopCode(t *testing.T) {
	dst := make([]float32, 1)
	Decode(dst, []byte{0xff, 0xff, 0xff, 0x7f}, I32LE)
	if dst[0] != 1 {
		t.Errorf("top I32 code into float32 = %v, want 1", dst[0])
	}
}

func TestEncodeRule(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		f    Format
		src  []float64
		want []byte
	}{
		// +1 is the top code, past either rail clips, NaN is zero.
		{U8, []float64{-1, 0, 1, 2, -2, nan, math.Copysign(0, -1)}, []byte{0, 128, 255, 255, 0, 128, 128}},
		{I8, []float64{-1, 0.5, 1, 1e9, math.Inf(-1), nan}, []byte{0x80, 0x40, 0x7f, 0x7f, 0x80, 0}},
		{I16LE, []float64{-1, 1, math.Inf(1), nan}, []byte{0x00, 0x80, 0xff, 0x7f, 0xff, 0x7f, 0, 0}},
		{I16BE, []float64{-1, 1, 1.0 / 32768}, []byte{0x80, 0x00, 0x7f, 0xff, 0x00, 0x01}},
		{I24LE, []float64{-1, 1}, []byte{0x00, 0x00, 0x80, 0xff, 0xff, 0x7f}},
		{I24BE, []float64{-1.0 / (1 << 23)}, []byte{0xff, 0xff, 0xff}},
		{I32LE, []float64{-1, 1, nan}, []byte{0, 0, 0, 0x80, 0xff, 0xff, 0xff, 0x7f, 0, 0, 0, 0}},
		{I32BE, []float64{-1}, []byte{0x80, 0, 0, 0}},
		// Floats pass through, past full scale included.
		{F32LE, []float64{2}, binary.LittleEndian.AppendUint32(nil, math.Float32bits(2))},
		{F32BE, []float64{-3}, binary.BigEndian.AppendUint32(nil, math.Float32bits(-3))},
		{F64LE, []float64{1e300}, binary.LittleEndian.AppendUint64(nil, math.Float64bits(1e300))},
		{F64BE, []float64{-0.1}, binary.BigEndian.AppendUint64(nil, math.Float64bits(-0.1))},
	} {
		got := make([]byte, len(tc.want))
		if n := Encode(got, tc.src, tc.f); n != len(tc.src) || !bytes.Equal(got, tc.want) {
			t.Errorf("%v: Encode(%v) = %d % x, want % x", tc.f, tc.src, n, got, tc.want)
		}
		src32 := make([]float32, len(tc.src))
		for i, v := range tc.src {
			src32[i] = float32(v)
		}
		got32 := make([]byte, len(tc.want))
		want32 := make([]byte, len(tc.want))
		Encode(got32, src32, tc.f)
		Encode(want32, widen(src32), tc.f)
		if !bytes.Equal(got32, want32) {
			t.Errorf("%v: Encode float32 = % x, float64 of the same values % x", tc.f, got32, want32)
		}
	}
}

func widen(s []float32) []float64 {
	out := make([]float64, len(s))
	for i, v := range s {
		out[i] = float64(v)
	}
	return out
}

// TestEncodeTiesToEven checks that a sample exactly between two codes takes
// the even one, at both widths and on both sides of zero.
func TestEncodeTiesToEven(t *testing.T) {
	for _, tc := range []struct {
		code float64 // in units of the format's step
		want int
	}{
		{0.5, 0}, {1.5, 2}, {2.5, 2}, {-0.5, 0}, {-1.5, -2}, {-2.5, -2},
		{0.49999, 0}, {0.50001, 1}, {32766.5, 32766}, {-32767.5, -32768},
	} {
		v := tc.code / 32768
		for _, s := range []any{[]float32{float32(v)}, []float64{v}} {
			out := make([]byte, 2)
			switch s := s.(type) {
			case []float32:
				Encode(out, s, I16LE)
			case []float64:
				Encode(out, s, I16LE)
			}
			if got := int(int16(binary.LittleEndian.Uint16(out))); got != tc.want {
				t.Errorf("%T %v codes: got %d, want %d", s, tc.code, got, tc.want)
			}
		}
	}
	// The tie that only a float64 can hold at 32 bits.
	out := make([]byte, 4)
	Encode(out, []float64{2.5 / (1 << 31)}, I32LE)
	if got := int32(binary.LittleEndian.Uint32(out)); got != 2 {
		t.Errorf("I32 tie: got %d, want 2", got)
	}
}

// TestRoundTripEveryCode decodes every code of the narrow formats and a sweep
// of the wide ones and encodes it back.
func TestRoundTripEveryCode(t *testing.T) {
	for _, f := range []Format{U8, I8, I16LE, I16BE} {
		src := make([]byte, (1<<f.Bits())*f.Width())
		for c := range 1 << f.Bits() {
			for j := range f.Width() {
				src[c*f.Width()+j] = byte(c >> (8 * j))
			}
		}
		roundTrip[float32](t, f, src)
		roundTrip[float64](t, f, src)
		roundTrip[complex64](t, f, src)
		roundTrip[complex128](t, f, src)
	}

	rng := rand.New(rand.NewPCG(3, 4))
	for _, f := range []Format{I24LE, I24BE, I32LE, I32BE} {
		src := make([]byte, 1<<16*f.Width())
		fillBytes(rng, src)
		if f.Bits() <= 24 {
			roundTrip[float32](t, f, src)
			roundTrip[complex64](t, f, src)
		}
		roundTrip[float64](t, f, src)
		roundTrip[complex128](t, f, src)
	}
}

func roundTrip[T dsp.Sample](t *testing.T, f Format, src []byte) {
	t.Helper()
	dst := make([]T, len(src)/f.Width()/elems[T]())
	Decode(dst, src, f)
	out := make([]byte, len(src))
	Encode(out, dst, f)
	if !bytes.Equal(out, src) {
		for i := range out {
			if out[i] != src[i] {
				t.Fatalf("%v %T: byte %d round-trips to %#x, want %#x", f, dst, i, out[i], src[i])
			}
		}
	}
}

// TestPartialSample checks the count rule at both ends: a trailing partial
// sample in src, and a dst too short for the next whole one, are left alone.
func TestPartialSample(t *testing.T) {
	for _, f := range Formats() {
		w := f.Width()
		src := make([]byte, 3*w+w-1)
		dst := make([]float64, 10)
		if n := Decode(dst, src, f); n != 3 {
			t.Errorf("%v: Decode of 3.x samples = %d", f, n)
		}
		cdst := make([]complex64, 10)
		if n := Decode(cdst, src, f); n != 1 {
			t.Errorf("%v: complex Decode of 1.x pairs = %d", f, n)
		}

		out := bytes.Repeat([]byte{0xee}, 2*w+w-1)
		if n := Encode(out, []float32{0.5, 0.5, 0.5, 0.5}, f); n != 2 {
			t.Errorf("%v: Encode into 2.x samples = %d", f, n)
		}
		if !bytes.Equal(out[2*w:], bytes.Repeat([]byte{0xee}, w-1)) {
			t.Errorf("%v: Encode wrote past its last whole sample: % x", f, out[2*w:])
		}
		if n := Encode(out, []complex128{0.5, 0.5}, f); n != 1 {
			t.Errorf("%v: complex Encode into 2.x elements = %d", f, n)
		}
	}
}

func TestInvalidFormat(t *testing.T) {
	for _, f := range []Format{0, Format(99), -1} {
		if n := Decode(make([]float32, 4), make([]byte, 16), f); n != 0 {
			t.Errorf("Decode %v = %d", f, n)
		}
		if n := Decode(make([]complex128, 4), make([]byte, 16), f); n != 0 {
			t.Errorf("Decode complex %v = %d", f, n)
		}
		if n := Encode(make([]byte, 16), make([]float64, 4), f); n != 0 {
			t.Errorf("Encode %v = %d", f, n)
		}
		if n := Encode(make([]byte, 16), make([]complex64, 4), f); n != 0 {
			t.Errorf("Encode complex %v = %d", f, n)
		}
	}
}

// TestComplexIsInterleavedReal checks a complex decode and encode against the
// real path over the same bytes.
func TestComplexIsInterleavedReal(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for _, f := range Formats() {
		src := make([]byte, 37*2*f.Width())
		fillBytes(rng, src)
		re := make([]float32, 74)
		Decode(re, src, f)
		c := make([]complex64, 37)
		Decode(c, src, f)
		for i, v := range c {
			if !sameFloat(real(v), re[2*i]) || !sameFloat(imag(v), re[2*i+1]) {
				t.Fatalf("%v: complex[%d] = %v, real pair %v %v", f, i, v, re[2*i], re[2*i+1])
			}
		}
		a, b := make([]byte, len(src)), make([]byte, len(src))
		Encode(a, re, f)
		Encode(b, c, f)
		if !bytes.Equal(a, b) {
			t.Errorf("%v: complex Encode differs from real", f)
		}
	}
}

// TestBigEndianIsReversed checks each big-endian format against its
// little-endian twin with every sample's bytes reversed, over more samples than
// one block of the reversal buffer.
func TestBigEndianIsReversed(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	for _, f := range Formats() {
		if !f.bigEndian() {
			continue
		}
		le := littleEndianOf(f)
		w := f.Width()
		n := 3*reverseBlock/w + 5
		be := make([]byte, n*w)
		fillBytes(rng, be)
		rev := make([]byte, len(be))
		reverseInto(rev, be, n, w)

		a, b := make([]float64, n), make([]float64, n)
		Decode(a, be, f)
		Decode(b, rev, le)
		for i := range a {
			if !sameFloat(a[i], b[i]) {
				t.Fatalf("%v: sample %d = %v, %v reversed gives %v", f, i, a[i], le, b[i])
			}
		}

		src := make([]float32, n)
		fillEncode(rng, src, w)
		x, y := make([]byte, n*w), make([]byte, n*w)
		Encode(x, src, f)
		Encode(y, src, le)
		reverseInto(rev, y, n, w)
		if !bytes.Equal(x, rev) {
			t.Errorf("%v: Encode is not %v reversed", f, le)
		}
	}
}

func littleEndianOf(f Format) Format {
	switch f {
	case I16BE:
		return I16LE
	case I24BE:
		return I24LE
	case I32BE:
		return I32LE
	case F32BE:
		return F32LE
	case F64BE:
		return F64LE
	}
	return f
}

func TestFloatFormatsKeepNaN(t *testing.T) {
	for _, f := range []Format{F32LE, F32BE, F64LE, F64BE} {
		out := make([]byte, 2*f.Width())
		Encode(out, []float64{math.NaN(), math.Inf(-1)}, f)
		back := make([]float64, 2)
		Decode(back, out, f)
		if !math.IsNaN(back[0]) || !math.IsInf(back[1], -1) {
			t.Errorf("%v: NaN and -Inf round-trip to %v", f, back)
		}
	}
}

func TestFormatFloat(t *testing.T) {
	for _, f := range Formats() {
		want := f == F32LE || f == F32BE || f == F64LE || f == F64BE
		if f.Float() != want {
			t.Errorf("%v.Float() = %v", f, f.Float())
		}
	}
	if Format(0).Float() {
		t.Error("the zero Format reports itself float")
	}
}

func ExampleDecode() {
	raw := []byte{0, 64, 128, 192, 255} // u8, as an rtl-sdr delivers
	samples := make([]float32, len(raw))
	n := Decode(samples, raw, U8)
	fmt.Println(n, samples)
	// Output:
	// 5 [-1 -0.5 0 0.5 0.9921875]
}

func ExampleEncode() {
	samples := []float64{-1, -0.5, 0.25 / 32768, 0.5, 1, 2, math.NaN()}
	raw := make([]byte, 2*len(samples))
	Encode(raw, samples, I16LE)
	for i := range samples {
		fmt.Print(int16(binary.LittleEndian.Uint16(raw[2*i:])), " ")
	}
	fmt.Println()
	// Output:
	// -32768 -16384 0 16384 32767 32767 0
}
