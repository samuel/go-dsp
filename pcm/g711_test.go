package pcm

import (
	"math"
	"testing"
)

// TestG711Levels checks the expansion against values from the tables of
// G.711: the first sixteen mu-law codes and the ends of each law.
func TestG711Levels(t *testing.T) {
	for i, want := range []int16{
		-32124, -31100, -30076, -29052, -28028, -27004, -25980, -24956,
		-23932, -22908, -21884, -20860, -19836, -18812, -17788, -16764,
	} {
		if got := mulawLevels[i]; got != want {
			t.Errorf("mu-law %#02x = %d, want %d", i, got, want)
		}
	}
	for code, want := range map[byte]int16{0x00: -32124, 0x7f: 0, 0x80: 32124, 0xff: 0} {
		if got := mulawLevels[code]; got != want {
			t.Errorf("mu-law %#02x = %d, want %d", code, got, want)
		}
	}
	for code, want := range map[byte]int16{0x00: -5504, 0x2a: -32256, 0x55: -8, 0xaa: 32256, 0xd5: 8} {
		if got := alawLevels[code]; got != want {
			t.Errorf("A-law %#02x = %d, want %d", code, got, want)
		}
	}
	// Each law is odd: flipping the sign bit negates the level.
	for code := range 128 {
		if alawLevels[code] != -alawLevels[code|0x80] {
			t.Errorf("A-law %#02x and %#02x are not mirrored", code, code|0x80)
		}
		if mulawLevels[code] != -mulawLevels[code|0x80] {
			t.Errorf("mu-law %#02x and %#02x are not mirrored", code, code|0x80)
		}
	}
}

// g711Step returns the width of a code's step on the 16-bit scale: 16 in
// A-law's first two segments and 8 in mu-law's first, doubling with each
// segment after.
func g711Step(f Format, code byte) int32 {
	if f == MuLaw {
		return 8 << (^code & 0x70 >> 4)
	}
	seg := (code ^ 0x55) & 0x70 >> 4
	return 16 << max(seg, 1) >> 1
}

// inG711Step reports whether q lies in the step of code, whose level is its
// center: no further below the level than half the step, and less than half
// above. Past the top step everything belongs to it.
func inG711Step(f Format, code byte, q int32) bool {
	levels := alawLevels
	if f == MuLaw {
		levels = mulawLevels
	}
	l, a, half := int32(levels[code]), q, g711Step(f, code)/2
	if l < 0 || (l == 0 && code&0x80 == 0) {
		l, a = -l, -a
	}
	if a < 0 {
		return false
	}
	top := int32(max(levels[0x80], levels[0xaa]))
	return (l-a <= half && a-l < half) || (l == top && a > l)
}

// TestG711CompressFindsTheStep checks the encode rule over every 16-bit
// sample: the code is the one whose step holds the sample, on the sample's
// side of zero, and the levels never decrease as the sample rises. The edges
// of the steps are the decision values of G.711.
func TestG711CompressFindsTheStep(t *testing.T) {
	for _, law := range []struct {
		f        Format
		levels   *[256]int16
		compress func(int32) byte
	}{
		{ALaw, alawLevels, alawCompress},
		{MuLaw, mulawLevels, mulawCompress},
	} {
		prev := int32(math.MinInt32)
		for q := int32(math.MinInt16); q <= math.MaxInt16; q++ {
			code := law.compress(q)
			got := int32(law.levels[code])
			if !inG711Step(law.f, code, q) {
				t.Fatalf("%v: %d encodes to %#02x, level %d, whose step does not hold it", law.f, q, code, got)
			}
			if got < prev {
				t.Fatalf("%v: %d encodes to level %d, below the %d before it", law.f, q, got, prev)
			}
			prev = got
		}
	}
	// The first sample of a segment belongs to it, though the last level of
	// the segment below is nearer: the decision value is the edge of the step,
	// not the midpoint between levels.
	if got := alawLevels[alawCompress(16384)]; got != 16896 {
		t.Errorf("A-law 16384 encodes to level %d, want 16896", got)
	}
	if got := mulawLevels[mulawCompress(16384-mulawBias)]; got != 16896-mulawBias {
		t.Errorf("mu-law %d encodes to level %d, want %d", 16384-mulawBias, got, 16896-mulawBias)
	}
	// The sign of a mu-law zero follows the sample, and A-law's zero is its
	// smallest positive level.
	if got := mulawCompress(-1); got != 0x7f {
		t.Errorf("mu-law -1 encodes to %#02x, want 0x7f", got)
	}
	if got := mulawCompress(0); got != 0xff {
		t.Errorf("mu-law 0 encodes to %#02x, want 0xff", got)
	}
	if got := alawCompress(0); got != 0xd5 {
		t.Errorf("A-law 0 encodes to %#02x, want 0xd5", got)
	}
}

// TestG711EncodeIsCompressedI16 is the definition of the encoders: the 16-bit
// quantizer, then the compressor.
func TestG711EncodeIsCompressedI16(t *testing.T) {
	// Between the 16-bit codes too, where the quantizer has to round.
	src := []float32{0.25 / 32768, 0.5 / 32768, 1.5 / 32768, -0.5 / 32768, -2.5 / 32768, 0.75}
	for i := range 1 << 16 {
		src = append(src, float32(i-1<<15)/(1<<15))
	}
	i16 := make([]byte, 2*len(src))
	Encode(i16, src, I16LE)
	for _, law := range []struct {
		f        Format
		compress func(int32) byte
	}{{ALaw, alawCompress}, {MuLaw, mulawCompress}} {
		got := make([]byte, len(src))
		Encode(got, src, law.f)
		got64 := make([]byte, len(src))
		Encode(got64, widen(src), law.f)
		for i := range src {
			q := int32(int16(uint16(i16[2*i]) | uint16(i16[2*i+1])<<8))
			if want := law.compress(q); got[i] != want || got64[i] != want {
				t.Fatalf("%v: %v encodes to %#02x (float64 %#02x), want %#02x", law.f, src[i], got[i], got64[i], want)
			}
		}
	}
}

// TestMuLawRoundTripEveryCode decodes and re-encodes every code. Mu-law has
// two zeros, and the negative one comes back as the positive. A-law is in
// TestRoundTripEveryCode.
func TestMuLawRoundTripEveryCode(t *testing.T) {
	src := make([]byte, 256)
	for i := range src {
		src[i] = byte(i)
	}
	x32, x64 := make([]float32, 256), make([]float64, 256)
	Decode(x32, src, MuLaw)
	Decode(x64, src, MuLaw)
	out32, out64 := make([]byte, 256), make([]byte, 256)
	Encode(out32, x32, MuLaw)
	Encode(out64, x64, MuLaw)
	for i := range src {
		want := src[i]
		if want == 0x7f {
			want = 0xff
		}
		if out32[i] != want || out64[i] != want {
			t.Errorf("mu-law %#02x round-trips to %#02x (float64 %#02x), want %#02x", src[i], out32[i], out64[i], want)
		}
	}
}

func TestFormatCompanded(t *testing.T) {
	for _, f := range Formats() {
		if want := f == ALaw || f == MuLaw; f.Companded() != want {
			t.Errorf("%v.Companded() = %v", f, f.Companded())
		}
	}
	if Format(0).Companded() {
		t.Error("the zero Format reports itself companded")
	}
}
