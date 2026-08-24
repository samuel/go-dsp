package sampleio

import (
	"math"
	"testing"

	"github.com/samuel/go-dsp/dsp/encoding"
)

// TestDecodeRails is the full-scale contract, written as literal codes rather
// than as the formula the code uses. Every decibel measured downstream is
// relative to this, so a full-scale sine has to read exactly 0 dBFS.
func TestDecodeRails(t *testing.T) {
	for _, tc := range []struct {
		f    Format
		src  []byte
		want []float64
	}{
		// 8-bit unsigned: 0 is the bottom rail, 128 is the middle.
		{U8, []byte{0x00, 0x80, 0xff, 0xc0, 0x40}, []float64{-1, 0, 127.0 / 128, 0.5, -0.5}},
		// 8-bit signed.
		{I8, []byte{0x80, 0x00, 0x7f, 0x40, 0xc0}, []float64{-1, 0, 127.0 / 128, 0.5, -0.5}},

		// 16-bit: -32768 -> -1, 16384 -> 0.5, 32767 -> just short of 1.
		{I16LE, []byte{0x00, 0x80, 0x00, 0x00, 0xff, 0x7f, 0x00, 0x40, 0x00, 0xc0},
			[]float64{-1, 0, 32767.0 / 32768, 0.5, -0.5}},
		{I16BE, []byte{0x80, 0x00, 0x00, 0x00, 0x7f, 0xff, 0x40, 0x00, 0xc0, 0x00},
			[]float64{-1, 0, 32767.0 / 32768, 0.5, -0.5}},

		// 24-bit packed, three bytes per sample and not padded to four.
		{I24LE, []byte{0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x40},
			[]float64{-1, 0, 8388607.0 / 8388608, 0.5}},
		{I24BE, []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7f, 0xff, 0xff, 0x40, 0x00, 0x00},
			[]float64{-1, 0, 8388607.0 / 8388608, 0.5}},

		{I32LE, []byte{0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40},
			[]float64{-1, 0, 0.5}},
		{I32BE, []byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00},
			[]float64{-1, 0, 0.5}},

		// A float file is already in these units and is passed through.
		{F32LE, []byte{0x00, 0x00, 0x80, 0xbf, 0x00, 0x00, 0x00, 0x3f}, []float64{-1, 0.5}},
		{F32BE, []byte{0xbf, 0x80, 0x00, 0x00, 0x3f, 0x00, 0x00, 0x00}, []float64{-1, 0.5}},
		{F64LE, []byte{0, 0, 0, 0, 0, 0, 0xf0, 0xbf, 0, 0, 0, 0, 0, 0, 0xe0, 0x3f}, []float64{-1, 0.5}},
		{F64BE, []byte{0xbf, 0xf0, 0, 0, 0, 0, 0, 0, 0x3f, 0xe0, 0, 0, 0, 0, 0, 0}, []float64{-1, 0.5}},
	} {
		t.Run(tc.f.String(), func(t *testing.T) {
			got := make([]float64, len(tc.want))
			var swap []byte
			if n := tc.f.decode(got, tc.src, &swap); n != len(tc.want) {
				t.Fatalf("decoded %d samples, want %d", n, len(tc.want))
			}
			for i := range got {
				// Exact: every divisor is a power of two, so there is no
				// rounding to allow for.
				if got[i] != tc.want[i] {
					t.Errorf("sample %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestDecodeEveryFormatHasRails walks every format rather than the table above,
// so a format added without a test cannot slip through.
func TestDecodeEveryFormatHasRails(t *testing.T) {
	for _, f := range Formats() {
		width := f.Width()
		if width == 0 {
			t.Errorf("%v has no width", f)
			continue
		}

		// The most negative code of an integer format, which must be exactly -1.
		var low []byte
		switch f {
		case U8:
			low = []byte{0x00}
		case I8:
			low = []byte{0x80}
		default:
			_, _, _, bits := f.info()
			if bits == 0 {
				continue // float formats have no code space to speak of
			}
			low = make([]byte, width)
			if _, big := f.littleEndian(); big {
				low[0] = 0x80
			} else {
				low[width-1] = 0x80
			}
		}
		got := make([]float64, 1)
		var swap []byte
		if n := f.decode(got, low, &swap); n != 1 {
			t.Fatalf("%v: decoded %d samples", f, n)
		}
		if got[0] != -1 {
			t.Errorf("%v: the most negative code read %v, want exactly -1", f, got[0])
		}
	}
}

// TestDecodeBigEndianMatchesLittle checks the byte reversal against the
// little-endian decoder it delegates to, which is the whole reason there is only
// one set of decoders.
func TestDecodeBigEndianMatchesLittle(t *testing.T) {
	for _, pair := range []struct{ be, le Format }{
		{I16BE, I16LE}, {I24BE, I24LE}, {I32BE, I32LE}, {F32BE, F32LE}, {F64BE, F64LE},
	} {
		width := pair.le.Width()
		const n = 17 // not a multiple of anything
		le := make([]byte, n*width)
		for i := range le {
			le[i] = byte(i*7 + 1)
		}
		be := make([]byte, len(le))
		for i := range n {
			for j := range width {
				be[i*width+j] = le[i*width+width-1-j]
			}
		}

		wantV := make([]float64, n)
		gotV := make([]float64, n)
		var s1, s2 []byte
		pair.le.decode(wantV, le, &s1)
		pair.be.decode(gotV, be, &s2)
		for i := range wantV {
			if gotV[i] != wantV[i] && !(math.IsNaN(gotV[i]) && math.IsNaN(wantV[i])) {
				t.Errorf("%v sample %d = %v, %v gave %v", pair.be, i, gotV[i], pair.le, wantV[i])
			}
		}
	}
}

// TestDecodeDoesNotTouchTheSource is what the byte reversal has to promise: a
// big-endian decode reverses into scratch, never into the caller's buffer.
func TestDecodeDoesNotTouchTheSource(t *testing.T) {
	src := []byte{0x80, 0x00, 0x7f, 0xff}
	keep := append([]byte(nil), src...)
	dst := make([]float64, 2)
	var swap []byte
	I16BE.decode(dst, src, &swap)
	for i := range src {
		if src[i] != keep[i] {
			t.Fatalf("decode modified its input at byte %d", i)
		}
	}
}

// TestDecodeShortInputs pins the two truncation rules: a trailing partial
// sample is not a sample, and a short destination stops the decode.
func TestDecodeShortInputs(t *testing.T) {
	// Five bytes is two whole 16-bit samples and a stray.
	src := []byte{0x00, 0x80, 0x00, 0x40, 0x11}
	dst := make([]float64, 4)
	var swap []byte
	if n := I16LE.decode(dst, src, &swap); n != 2 {
		t.Errorf("decoded %d samples from %d bytes, want 2", n, len(src))
	}
	if n := I16LE.decode(dst[:1], src, &swap); n != 1 {
		t.Errorf("decoded %d samples into a slice of 1", n)
	}
	if n := I16LE.decode(nil, src, &swap); n != 0 {
		t.Errorf("decoded %d samples into nil", n)
	}
	if n := I16LE.decode(dst, src[:1], &swap); n != 0 {
		t.Errorf("decoded %d samples from half a sample", n)
	}
	if n := Format(0).decode(dst, src, &swap); n != 0 {
		t.Errorf("the zero Format decoded %d samples", n)
	}
}

// TestDecodeNilScratch covers the documented one-shot path, where a caller that
// decodes once does not have to own a buffer.
func TestDecodeNilScratch(t *testing.T) {
	dst := make([]float64, 1)
	if n := I16BE.decode(dst, []byte{0x80, 0x00}, nil); n != 1 || dst[0] != -1 {
		t.Errorf("decode with nil scratch gave (%d, %v), want (1, -1)", n, dst[0])
	}
}

// BenchmarkDecode backs the claim in decode's doc comment: that reusing dsp's
// unscaled conversion and normalizing in a second pass costs little enough that
// a fused loop is not worth keeping a second set of decoders for.
func BenchmarkDecode(b *testing.B) {
	const n = 1 << 14
	src := make([]byte, n*2)
	for i := range src {
		src[i] = byte(i)
	}
	dst := make([]float64, n)
	var swap []byte

	b.Run("i16le/dsp+scale", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for b.Loop() {
			I16LE.decode(dst, src, &swap)
		}
	})
	b.Run("i16le/fused", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for b.Loop() {
			for i := range dst {
				dst[i] = float64(int16(uint16(src[i*2])|uint16(src[i*2+1])<<8)) * (1.0 / 32768)
			}
		}
	})
	b.Run("i16le/dsp-unscaled", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for b.Loop() {
			encoding.I16LEToF64(dst, src)
		}
	})
	b.Run("i16be/swap+dsp+scale", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		for b.Loop() {
			I16BE.decode(dst, src, &swap)
		}
	})
}
