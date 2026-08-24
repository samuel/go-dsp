package sampleio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// i16 packs samples the way a raw file holds them, so a test can say what it
// means without a fixture.
//
// It panics rather than wrapping on a value it cannot hold. Full scale is
// asymmetric -- the top code is 32767/32768, so +1 does not fit -- and a silent
// wrap turns a +1 in a test case into a -1, which then looks like a bug in the
// reader.
func i16(vals ...float64) []byte {
	b := make([]byte, 2*len(vals))
	for i, v := range vals {
		c := math.Round(v * 32768)
		if c < math.MinInt16 || c > math.MaxInt16 {
			panic(fmt.Sprintf("i16: %v is outside what int16 holds", v))
		}
		binary.LittleEndian.PutUint16(b[i*2:], uint16(int16(c)))
	}
	return b
}

// readAll drains a Reader, which is how every analysis in the tool will use it.
func readAll(t *testing.T, r *Reader, block int) []float64 {
	t.Helper()
	var out []float64
	buf := make([]float64, block)
	for {
		n, err := r.ReadFloat64(buf)
		out = append(out, buf[:n]...)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if n == 0 {
			t.Fatal("Read returned 0, nil, which would spin")
		}
	}
}

func TestReadRawMono(t *testing.T) {
	want := []float64{0, 0.5, -0.5, 0.25}
	r, err := NewReader(bytes.NewReader(i16(want...)), Options{Format: I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, r, 3) // a block that does not divide the length
	if len(got) != len(want) {
		t.Fatalf("read %d samples, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
	if info := r.Info(); info.Container != "raw" || info.Rate != 8000 || info.Channels != 1 {
		t.Errorf("Info = %+v", info)
	}
	// A stream has no length.
	if got := r.Info().Frames; got != -1 {
		t.Errorf("Frames = %d, want -1 for a stream", got)
	}
}

// TestReadBlockSizes is the property every streaming caller depends on: what
// comes out does not depend on how it was asked for.
func TestReadBlockSizes(t *testing.T) {
	want := make([]float64, 300)
	for i := range want {
		want[i] = float64(i%64)/64 - 0.5
	}
	raw := i16(want...)

	for _, block := range []int{1, 2, 3, 7, 64, 299, 300, 301, 4096} {
		r, err := NewReader(bytes.NewReader(raw), Options{Format: I16LE, Rate: 8000})
		if err != nil {
			t.Fatal(err)
		}
		got := readAll(t, r, block)
		if len(got) != len(want) {
			t.Errorf("block %d: read %d samples, want %d", block, len(got), len(want))
			continue
		}
		for i := range want {
			if got[i] != got[i] || got[i] != roundTrip16(want[i]) {
				t.Errorf("block %d: sample %d = %v", block, i, got[i])
				break
			}
		}
	}
}

func roundTrip16(v float64) float64 {
	return float64(int16(math.Round(v*32768))) / 32768
}

// TestReadTruncatedFrame is what a half-written capture looks like: a trailing
// partial frame is not a sample.
func TestReadTruncatedFrame(t *testing.T) {
	raw := append(i16(0.5, -0.5), 0x11) // two samples and a stray byte
	r, err := NewReader(bytes.NewReader(raw), Options{Format: I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, r, 8); len(got) != 2 {
		t.Errorf("read %d samples from two and a half, want 2", len(got))
	}
}

func TestReadChannels(t *testing.T) {
	// Two channels: the second is the negation of the first.
	raw := i16(0.5, -0.5, 0.25, -0.25, -0.75, 0.75)

	for _, tc := range []struct {
		name string
		pick Channel
		want []float64
	}{
		{"left", 0, []float64{0.5, 0.25, -0.75}},
		{"right", 1, []float64{-0.5, -0.25, 0.75}},
		// The channels cancel exactly, so a mixdown is silence -- which is a
		// stronger check than a plausible-looking average.
		{"mix", MixChannels, []float64{0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(raw), Options{
				Format: I16LE, Rate: 8000, Channels: 2, Channel: tc.pick,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := readAll(t, r, 2)
			if len(got) != len(tc.want) {
				t.Fatalf("read %d frames, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("frame %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestReadComplex128(t *testing.T) {
	// Interleaved I/Q: (0.5, -0.5), (0.25, 0.75).
	raw := i16(0.5, -0.5, 0.25, 0.75)
	r, err := NewReader(bytes.NewReader(raw), Options{Format: I16LE, Rate: 8000, IQ: true})
	if err != nil {
		t.Fatal(err)
	}
	if info := r.Info(); !info.Complex || info.Channels != 2 {
		t.Errorf("Info = %+v, want a 2-channel complex stream", info)
	}

	dst := make([]complex128, 4)
	n, err := r.ReadComplex128(dst)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	want := []complex128{complex(0.5, -0.5), complex(0.25, 0.75)}
	if n != len(want) {
		t.Fatalf("read %d I/Q samples, want %d", n, len(want))
	}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("sample %d = %v, want %v", i, dst[i], want[i])
		}
	}
}

// TestReadWrongShape validates that a complex stream does not hand back its
// real part, nor a real stream pretend to be complex.
func TestReadWrongShape(t *testing.T) {
	iq, err := NewReader(bytes.NewReader(i16(0, 0, 0, 0)), Options{Format: I16LE, Rate: 8000, IQ: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iq.ReadFloat64(make([]float64, 2)); err == nil {
		t.Error("Read on an I/Q stream succeeded")
	}

	realRd, err := NewReader(bytes.NewReader(i16(0, 0)), Options{Format: I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := realRd.ReadComplex128(make([]complex128, 2)); err == nil {
		t.Error("ReadComplex128 on a real stream succeeded")
	}
}

// TestReadComplexPairs checks that with four interleaved channels the pair index
// selects an I/Q pair rather than a channel.
func TestReadComplexPairs(t *testing.T) {
	// Frame: I0 Q0 I1 Q1.
	raw := i16(0.5, 0.25, -0.5, -0.25)
	for pair, want := range map[Channel]complex128{
		0: complex(0.5, 0.25),
		1: complex(-0.5, -0.25),
	} {
		r, err := NewReader(bytes.NewReader(raw), Options{
			Format: I16LE, Rate: 8000, Channels: 4, Channel: pair, IQ: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		dst := make([]complex128, 2)
		n, err := r.ReadComplex128(dst)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		if n != 1 || dst[0] != want {
			t.Errorf("pair %d: read %d samples, first %v, want 1 and %v", pair, n, dst[0], want)
		}
	}
}

func TestReadSkipAndLimit(t *testing.T) {
	all := []float64{0, 0.125, 0.25, 0.375, 0.5, 0.625, 0.75, 0.875}
	raw := i16(all...)

	for _, tc := range []struct {
		name        string
		skip, limit int64
		want        []float64
	}{
		{"skip", 3, 0, all[3:]},
		{"limit", 0, 3, all[:3]},
		{"both", 2, 3, all[2:5]},
		{"skip past the end", 100, 0, nil},
		{"limit past the end", 0, 100, all},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(raw), Options{
				Format: I16LE, Rate: 8000, Skip: tc.skip, Limit: tc.limit,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := readAll(t, r, 3)
			if len(got) != len(tc.want) {
				t.Fatalf("read %d samples, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("sample %d = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestOpenRawReportsItsLength is what lets a caller choose a frame count before
// reading anything, which is how the spectrogram picks its hop.
func TestOpenRawReportsItsLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.raw")
	if err := os.WriteFile(path, i16(make([]float64, 100)...), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name        string
		opt         Options
		wantFrames  int64
		wantSeconds float64
	}{
		{"whole file", Options{Format: I16LE, Rate: 100}, 100, 1},
		{"stereo halves it", Options{Format: I16LE, Rate: 100, Channels: 2}, 50, 0.5},
		{"skip", Options{Format: I16LE, Rate: 100, Skip: 40}, 60, 0.6},
		{"limit", Options{Format: I16LE, Rate: 100, Limit: 10}, 10, 0.1},
		{"skip and limit", Options{Format: I16LE, Rate: 100, Skip: 95, Limit: 10}, 5, 0.05},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Open(path, tc.opt)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
			if got := r.Info().Frames; got != tc.wantFrames {
				t.Errorf("Frames = %d, want %d", got, tc.wantFrames)
			}
			if got := r.Info().Duration(); math.Abs(got-tc.wantSeconds) > 1e-12 {
				t.Errorf("Duration = %v, want %v", got, tc.wantSeconds)
			}
			// The promise has to hold: Frames is what actually comes out.
			if got := int64(len(readAll(t, r, 7))); got != tc.wantFrames {
				t.Errorf("read %d samples but Frames said %d", got, tc.wantFrames)
			}
		})
	}
}

func TestReaderRejectsBadOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  Options
	}{
		{"no format", Options{Rate: 8000}},
		{"no rate", Options{Format: I16LE}},
		{"invalid format", Options{Format: Format(99), Rate: 8000}},
		{"negative skip", Options{Format: I16LE, Rate: 8000, Skip: -1}},
		{"negative limit", Options{Format: I16LE, Rate: 8000, Limit: -1}},
		{"channel out of range", Options{Format: I16LE, Rate: 8000, Channels: 2, Channel: 2}},
		{"negative channel", Options{Format: I16LE, Rate: 8000, Channel: -7}},
		{"mixing an I/Q stream", Options{Format: I16LE, Rate: 8000, IQ: true, Channel: MixChannels}},
		{"odd channels for I/Q", Options{Format: I16LE, Rate: 8000, IQ: true, Channels: 3}},
		{"I/Q pair out of range", Options{Format: I16LE, Rate: 8000, IQ: true, Channels: 2, Channel: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewReader(bytes.NewReader(make([]byte, 64)), tc.opt); err == nil {
				t.Error("accepted options that should be rejected")
			}
		})
	}
}

// TestReadFloat32 covers the width the filters in dsp take, which is what the
// examples need.
func TestReadFloat32(t *testing.T) {
	r, err := NewReader(bytes.NewReader(i16(0.5, -0.5, 0.25)), Options{Format: I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 4)
	n, err := r.ReadFloat32(dst)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	want := []float32{0.5, -0.5, 0.25}
	if n != len(want) {
		t.Fatalf("read %d samples, want %d", n, len(want))
	}
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("sample %d = %v, want %v", i, dst[i], want[i])
		}
	}
}

// TestReadEOFIsSticky makes sure a drained Reader keeps saying so rather than
// blocking or spinning.
func TestReadEOFIsSticky(t *testing.T) {
	r, err := NewReader(bytes.NewReader(i16(0.5)), Options{Format: I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, r, 4)
	for range 3 {
		if n, err := r.ReadFloat64(make([]float64, 4)); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("after EOF, Read = (%d, %v), want (0, EOF)", n, err)
		}
	}
}

// TestReadFloat32Mixdown is the regression for a scratch buffer that was also a
// destination: ReadFloat32 used to hand Read its own work buffer to fill, and
// decodeReal's mixdown branch uses work as its interleaved scratch and
// reallocates it when the frame count outgrows it, after which the conversion
// loop reads raw interleaved samples instead of averaged ones. Three frames of
// two channels needs six float64 of scratch against a three-element request, so
// the reallocation happens and the bug shows.
func TestReadFloat32Mixdown(t *testing.T) {
	raw := i16(0.5, -0.5, 0.25, -0.25, -0.75, 0.75)
	r, err := NewReader(bytes.NewReader(raw), Options{
		Format: I16LE, Rate: 8000, Channels: 2, Channel: MixChannels,
	})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 3)
	n, err := r.ReadFloat32(dst)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("read %d frames, want 3", n)
	}
	// Every frame's two channels cancel exactly.
	for i, v := range dst {
		if v != 0 {
			t.Errorf("frame %d mixes to %v, want 0 (a raw channel value here means "+
				"the conversion read a reallocated scratch buffer)", i, v)
		}
	}
}

// readAllPlanar drains a Reader into one slice per channel, the way a caller
// that wants every channel would.
func readAllPlanar(t *testing.T, r *Reader, channels, block int) [][]float64 {
	t.Helper()
	out := make([][]float64, channels)
	planes := make([][]float64, channels)
	for c := range planes {
		planes[c] = make([]float64, block)
	}
	for {
		n, err := r.ReadFloat64Planar(planes)
		for c := range out {
			out[c] = append(out[c], planes[c][:n]...)
		}
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("ReadFloat64Planar: %v", err)
		}
		if n == 0 {
			t.Fatal("ReadFloat64Planar returned 0, nil, which would spin")
		}
	}
}

// TestReadFloat64PlanarMatchesPerChannelReaders pins that reading every
// channel at once produces exactly what a Reader per channel
// produces, bit for bit. The two decode the same bytes with the same arithmetic
// in the same order, so equality is the right comparison.
//
// The oracle being another Reader lets the input be arbitrary bytes, which
// covers all twelve formats, both byte orders and the packed three-byte width
// for nothing.
func TestReadFloat64PlanarMatchesPerChannelReaders(t *testing.T) {
	blocks := []int{1, 2, 3, 7, 64, 299, 300, 301, 4096}
	for _, format := range Formats() {
		for _, channels := range []int{1, 2, 3, 8} {
			const frames = 300
			raw := make([]byte, frames*channels*format.Width())
			for i := range raw {
				raw[i] = byte(i*31 + 7)
			}
			opt := Options{Format: format, Rate: 8000, Channels: channels, Raw: true}

			// The oracle: one Reader per channel over the same bytes.
			want := make([][]float64, channels)
			for c := range want {
				o := opt
				o.Channel = Channel(c)
				r, err := NewReader(bytes.NewReader(raw), o)
				if err != nil {
					t.Fatal(err)
				}
				want[c] = readAll(t, r, 64)
			}

			for _, block := range blocks {
				name := fmt.Sprintf("%v/%dch/block%d", format, channels, block)
				t.Run(name, func(t *testing.T) {
					r, err := NewReader(bytes.NewReader(raw), opt)
					if err != nil {
						t.Fatal(err)
					}
					got := readAllPlanar(t, r, channels, block)
					for c := range got {
						if len(got[c]) != len(want[c]) {
							t.Fatalf("channel %d: %d frames, want %d", c, len(got[c]), len(want[c]))
						}
						for i := range want[c] {
							// Arbitrary bytes read as a float format can be NaN,
							// which is never equal to itself.
							if got[c][i] != want[c][i] &&
								!(math.IsNaN(got[c][i]) && math.IsNaN(want[c][i])) {
								t.Fatalf("channel %d frame %d = %v, want %v",
									c, i, got[c][i], want[c][i])
							}
						}
					}
				})
			}
		}
	}
}

// TestReadFloat64PlanarMono covers the delegation to Read, which nothing else reaches:
// a one-channel file is already planar, so ReadFloat64Planar hands the plane straight
// to Read rather than staging and scattering it.
func TestReadFloat64PlanarMono(t *testing.T) {
	want := []float64{0, 0.5, -0.5, 0.25, -0.125}
	for _, block := range []int{1, 2, 3, 5, 64} {
		r, err := NewReader(bytes.NewReader(i16(want...)), Options{Format: I16LE, Rate: 8000})
		if err != nil {
			t.Fatal(err)
		}
		got := readAllPlanar(t, r, 1, block)
		if len(got) != 1 || len(got[0]) != len(want) {
			t.Fatalf("block %d: got %d planes of %d", block, len(got), len(got[0]))
		}
		for i := range want {
			if got[0][i] != want[i] {
				t.Errorf("block %d: frame %d = %v, want %v", block, i, got[0][i], want[i])
			}
		}
	}
}

// TestReadFloat64PlanarShortestPlaneWins pins the mismatched-length rule, and the half
// of it that makes the rule safe: the frames that did not fit were not consumed
// either, so the next call continues where this one stopped.
func TestReadFloat64PlanarShortestPlaneWins(t *testing.T) {
	// Codes 0..15 rather than amplitudes, so each frame says which one it is.
	raw := make([]byte, 32)
	for i := range 16 {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(int16(i)))
	}
	r, err := NewReader(bytes.NewReader(raw), Options{
		Format: I16LE, Rate: 8000, Channels: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	long, short := make([]float64, 5), make([]float64, 3)
	n, err := r.ReadFloat64Planar([][]float64{long, short})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("read %d frames, want 3, the shortest plane", n)
	}
	// Codes 0..15 interleaved as (left, right) per frame.
	for i := range 3 {
		wantL := float64(int16(2*i)) / 32768
		wantR := float64(int16(2*i+1)) / 32768
		if long[i] != wantL || short[i] != wantR {
			t.Errorf("frame %d = (%v, %v), want (%v, %v)", i, long[i], short[i], wantL, wantR)
		}
	}
	// The unwritten frames are still there.
	n, err = r.ReadFloat64Planar([][]float64{long, short})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("second read got %d frames, want 3", n)
	}
	if want := float64(int16(6)) / 32768; long[0] != want {
		t.Errorf("the second read starts at %v, want %v: the short plane consumed frames it did not write",
			long[0], want)
	}
}

// TestReadFloat64PlanarTruncatedFrame checks that a trailing partial frame is dropped
// rather than half-decoded, as TestReadTruncatedFrame does for one channel.
func TestReadFloat64PlanarTruncatedFrame(t *testing.T) {
	raw := append(i16(0.5, -0.5, 0.25, -0.25), 0x00) // two whole frames plus a stray byte
	r, err := NewReader(bytes.NewReader(raw), Options{Format: I16LE, Rate: 8000, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	got := readAllPlanar(t, r, 2, 4)
	if len(got[0]) != 2 || len(got[1]) != 2 {
		t.Fatalf("got %d and %d frames, want 2 each", len(got[0]), len(got[1]))
	}
}

// TestReadFloat64PlanarSkipAndLimit checks that both land on frames rather than on
// samples, which is the thing that goes wrong once a file has two channels.
func TestReadFloat64PlanarSkipAndLimit(t *testing.T) {
	raw := i16(0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7) // four stereo frames
	for _, tt := range []struct {
		name        string
		skip, limit int64
		wantL       []float64
	}{
		{"all", 0, 0, []float64{0, 0.2, 0.4, 0.6}},
		{"skip", 1, 0, []float64{0.2, 0.4, 0.6}},
		{"limit", 0, 2, []float64{0, 0.2}},
		{"both", 1, 2, []float64{0.2, 0.4}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(raw), Options{
				Format: I16LE, Rate: 8000, Channels: 2, Skip: tt.skip, Limit: tt.limit,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := readAllPlanar(t, r, 2, 3)
			if len(got[0]) != len(tt.wantL) {
				t.Fatalf("got %d frames, want %d", len(got[0]), len(tt.wantL))
			}
			for i, want := range tt.wantL {
				if math.Abs(got[0][i]-want) > 1.0/32768 {
					t.Errorf("frame %d = %v, want %v", i, got[0][i], want)
				}
			}
		})
	}
}

// TestReadFloat64PlanarIgnoresChannelSelection pins the surprising half of the
// contract: a planar read is the whole frame, so the channel the Reader was
// opened for does not narrow it.
func TestReadFloat64PlanarIgnoresChannelSelection(t *testing.T) {
	raw := i16(0.5, -0.5, 0.25, -0.25)
	r, err := NewReader(bytes.NewReader(raw), Options{
		Format: I16LE, Rate: 8000, Channels: 2, Channel: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := readAllPlanar(t, r, 2, 4)
	wantL := []float64{0.5, 0.25}
	wantR := []float64{-0.5, -0.25}
	for i := range wantL {
		if got[0][i] != wantL[i] || got[1][i] != wantR[i] {
			t.Errorf("frame %d = (%v, %v), want (%v, %v)", i, got[0][i], got[1][i], wantL[i], wantR[i])
		}
	}
}

// TestReadFloat64PlanarRejectsWrongShape checks the structural mistakes, which are
// loud, as against a mismatched plane length, which is not.
func TestReadFloat64PlanarRejectsWrongShape(t *testing.T) {
	planes := func(n, size int) [][]float64 {
		p := make([][]float64, n)
		for i := range p {
			p[i] = make([]float64, size)
		}
		return p
	}
	for _, tt := range []struct {
		name   string
		opt    Options
		planes int
	}{
		{"too few planes", Options{Format: I16LE, Rate: 8000, Channels: 2}, 1},
		{"too many planes", Options{Format: I16LE, Rate: 8000, Channels: 2}, 3},
		{"no planes", Options{Format: I16LE, Rate: 8000, Channels: 2}, 0},
		{"a mixdown", Options{Format: I16LE, Rate: 8000, Channels: 2, Channel: MixChannels}, 2},
		{"an I/Q stream", Options{Format: I16LE, Rate: 8000, Channels: 2, IQ: true}, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(i16(0, 0, 0, 0)), tt.opt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.ReadFloat64Planar(planes(tt.planes, 4)); err == nil {
				t.Error("ReadFloat64Planar: want an error")
			}
			f32 := make([][]float32, tt.planes)
			for i := range f32 {
				f32[i] = make([]float32, 4)
			}
			if _, err := r.ReadFloat32Planar(f32); err == nil {
				t.Error("ReadFloat32Planar: want an error")
			}
		})
	}
}

// TestReadFloat64PlanarEOFIsSticky makes sure a drained Reader keeps saying so rather
// than blocking or spinning, as TestReadEOFIsSticky does for one channel.
func TestReadFloat64PlanarEOFIsSticky(t *testing.T) {
	r, err := NewReader(bytes.NewReader(i16(0.5, -0.5)), Options{
		Format: I16LE, Rate: 8000, Channels: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	readAllPlanar(t, r, 2, 4)
	for range 3 {
		planes := [][]float64{make([]float64, 4), make([]float64, 4)}
		if n, err := r.ReadFloat64Planar(planes); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("after EOF, ReadFloat64Planar = (%d, %v), want (0, EOF)", n, err)
		}
	}
}

// TestReadFloat32PlanarMatchesPlanar checks that the narrowing happens and
// nothing else does: every value is exactly the float64 one converted.
func TestReadFloat32PlanarMatchesPlanar(t *testing.T) {
	for _, channels := range []int{1, 2, 3} {
		const frames = 200
		raw := make([]byte, frames*channels*2)
		for i := range raw {
			raw[i] = byte(i*13 + 3)
		}
		opt := Options{Format: I16LE, Rate: 8000, Channels: channels, Raw: true}

		r64, err := NewReader(bytes.NewReader(raw), opt)
		if err != nil {
			t.Fatal(err)
		}
		want := readAllPlanar(t, r64, channels, 7)

		r32, err := NewReader(bytes.NewReader(raw), opt)
		if err != nil {
			t.Fatal(err)
		}
		planes := make([][]float32, channels)
		for c := range planes {
			planes[c] = make([]float32, 7)
		}
		got := make([][]float32, channels)
		for {
			n, err := r32.ReadFloat32Planar(planes)
			for c := range got {
				got[c] = append(got[c], planes[c][:n]...)
			}
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		for c := range got {
			if len(got[c]) != len(want[c]) {
				t.Fatalf("%d channels: channel %d has %d frames, want %d",
					channels, c, len(got[c]), len(want[c]))
			}
			for i := range want[c] {
				if got[c][i] != float32(want[c][i]) {
					t.Errorf("%d channels: channel %d frame %d = %v, want %v",
						channels, c, i, got[c][i], float32(want[c][i]))
				}
			}
		}
	}
}

// BenchmarkReadPlanar backs the reason ReadFloat64Planar exists: one pass over the
// interleaved bytes for every channel, against one pass per channel.
//
// Both arms report the size of the file to SetBytes rather than the bytes each
// arm touches, so the MB/s figure answers "how fast does this deliver the samples
// in this file" -- which is the question, since the per-channel arm reads the
// same file once per channel. Raw keeps sniffWAV's bufio wrap out of both.
func BenchmarkReadPlanar(b *testing.B) {
	const frames = 1 << 16
	for _, channels := range []int{2, 4, 8} {
		raw := make([]byte, frames*channels*2)
		for i := range raw {
			raw[i] = byte(i*31 + 7)
		}
		opt := Options{Format: I16LE, Rate: 8000, Channels: channels, Raw: true}

		planes := make([][]float64, channels)
		for c := range planes {
			planes[c] = make([]float64, 4096)
		}
		one := make([]float64, 4096)

		b.Run(fmt.Sprintf("%dch/planar", channels), func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				r, err := NewReader(bytes.NewReader(raw), opt)
				if err != nil {
					b.Fatal(err)
				}
				for {
					if _, err := r.ReadFloat64Planar(planes); err != nil {
						break
					}
				}
			}
		})
		b.Run(fmt.Sprintf("%dch/readers", channels), func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for b.Loop() {
				for c := range channels {
					o := opt
					o.Channel = Channel(c)
					r, err := NewReader(bytes.NewReader(raw), o)
					if err != nil {
						b.Fatal(err)
					}
					for {
						if _, err := r.ReadFloat64(one); err != nil {
							break
						}
					}
				}
			}
		})
	}
}

// TestFloatOverFullScaleNote checks the Note the package comment promises for a
// float sample outside full scale. Every decibel measured downstream is relative
// to 1.0 and a float container does not enforce that: a peak of 2.0 reads as
// +6 dBFS, clipping that never happened. The note can appear only once the
// sample has been read, so Info is checked before and after.
func TestFloatOverFullScaleNote(t *testing.T) {
	raw := make([]byte, 0, 4*4)
	for _, v := range []float32{0.5, -0.25, 2.0, 0.1} {
		raw = binary.LittleEndian.AppendUint32(raw, math.Float32bits(v))
	}

	r, err := NewReader(io.NopCloser(bytes.NewReader(raw)), Options{
		Format: F32LE, Rate: 8000, Channels: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := noteAbout(r.Info().Notes, "full scale"); got != "" {
		t.Fatalf("the note appeared before anything was read: %q", got)
	}

	dst := make([]float64, 4)
	if n, err := r.ReadFloat64(dst); err != nil || n != 4 {
		t.Fatalf("ReadFloat64 = %d, %v", n, err)
	}
	if got := noteAbout(r.Info().Notes, "full scale"); got == "" {
		t.Errorf("no note about full scale in %q", r.Info().Notes)
	}

	// An integer format cannot produce one, since every code divides into
	// range by construction.
	ri, err := NewReader(io.NopCloser(bytes.NewReader([]byte{0x00, 0x80, 0xff, 0x7f})), Options{
		Format: I16LE, Rate: 8000, Channels: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ri.ReadFloat64(make([]float64, 2)); err != nil {
		t.Fatal(err)
	}
	if got := noteAbout(ri.Info().Notes, "full scale"); got != "" {
		t.Errorf("an integer stream produced %q", got)
	}
}

func noteAbout(notes []string, substr string) string {
	for _, n := range notes {
		if strings.Contains(n, substr) {
			return n
		}
	}
	return ""
}

// TestFillGivesUpOnAStalledReader checks that a source which only ever returns
// (0, nil) is reported rather than spun on. io.Reader permits that return, so
// the loop cannot simply trust it to make progress.
func TestFillGivesUpOnAStalledReader(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		// NewReader primes the buffer, so it is the first thing that can stall;
		// either it or the read has to come back with the error.
		r, err := NewReader(io.NopCloser(stalledReader{}), Options{Format: I16LE, Rate: 8000, Channels: 1})
		if err != nil {
			done <- err
			return
		}
		_, err = r.ReadFloat64(make([]float64, 16))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrNoProgress) {
			t.Errorf("got %v, want io.ErrNoProgress", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still spinning on a reader that never makes progress")
	}
}

type stalledReader struct{}

func (stalledReader) Read([]byte) (int, error) { return 0, nil }
