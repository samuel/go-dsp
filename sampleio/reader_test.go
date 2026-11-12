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
	"slices"
	"strings"
	"testing"
	"time"

	"samuelks.com/dsp"
	"samuelks.com/dsp/pcm"
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

// readAll reads a real stream to the end, block frames at a time. A stream of
// more than one channel gives up its first.
func readAll(t *testing.T, r *Reader, block int) []float64 {
	t.Helper()
	if n := r.Info().Channels; n > 1 {
		return drainPlane[float64](t, r, n, 0, block)
	}
	var out []float64
	buf := make([]float64, block)
	for {
		n, err := r.ReadSamples(buf)
		out = append(out, buf[:n]...)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("ReadSamples: %v", err)
		}
		if n == 0 {
			t.Fatal("ReadSamples returned 0, nil, which would spin")
		}
	}
}

func TestReadRawMono(t *testing.T) {
	want := []float64{0, 0.5, -0.5, 0.25}
	r, err := NewReader(bytes.NewReader(i16(want...)), Layout{Format: pcm.I16LE, Rate: 8000})
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
	if info := r.Info(); info.Container != Raw || info.Rate != 8000 || info.Channels != 1 {
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
		r, err := NewReader(bytes.NewReader(raw), Layout{Format: pcm.I16LE, Rate: 8000})
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
	r, err := NewReader(bytes.NewReader(raw), Layout{Format: pcm.I16LE, Rate: 8000})
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
	l := Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2}

	for c, want := range [][]float64{
		{0.5, 0.25, -0.75},
		{-0.5, -0.25, 0.75},
	} {
		r, err := NewReader(bytes.NewReader(raw), l)
		if err != nil {
			t.Fatal(err)
		}
		if got := drainPlane[float64](t, r, 2, c, 2); !slices.Equal(got, want) {
			t.Errorf("channel %d = %v, want %v", c, got, want)
		}
	}

	// ReadSamples has no way to say which channel, so it refuses.
	r, err := NewReader(bytes.NewReader(raw), l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadSamples(make([]float64, 2)); err == nil {
		t.Error("ReadSamples read a two-channel stream")
	}
}

func TestReadComplex(t *testing.T) {
	// Interleaved I/Q: (0.5, -0.5), (0.25, 0.75).
	raw := i16(0.5, -0.5, 0.25, 0.75)
	r, err := NewReader(bytes.NewReader(raw), Layout{Format: pcm.I16LE, Rate: 8000, IQ: true})
	if err != nil {
		t.Fatal(err)
	}
	if info := r.Info(); !info.IQ || info.Channels != 2 {
		t.Errorf("Info = %+v, want a 2-channel complex stream", info)
	}

	dst := make([]complex128, 4)
	n, err := r.ReadSamples(dst)
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

// TestReadComplex64 reads a stream longer than a raw block in one call, so
// the block loop runs more than once, and checks it against complex128
// narrowed.
func TestReadComplex64(t *testing.T) {
	vals := make([]float64, 2*20000)
	for i := range vals {
		vals[i] = float64(i%200-100) / 128
	}
	raw := i16(vals...)
	open := func() *Reader {
		r, err := NewReader(bytes.NewReader(raw), Layout{Format: pcm.I16LE, Rate: 8000, IQ: true})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	want := make([]complex128, len(vals)/2+5)
	nw, err := open().ReadSamples(want)
	if err != nil {
		t.Fatal(err)
	}
	r := open()
	got := make([]complex64, len(want))
	n, err := r.ReadSamples(got)
	if err != nil {
		t.Fatal(err)
	}
	if n != nw || n != len(vals)/2 {
		t.Fatalf("read %d, complex128 read %d, want %d", n, nw, len(vals)/2)
	}
	for i := range n {
		if got[i] != complex64(want[i]) {
			t.Fatalf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
	if _, err := r.ReadSamples(got); !errors.Is(err, io.EOF) {
		t.Errorf("read past the end: %v, want io.EOF", err)
	}
}

// TestReadWrongShape validates that a complex stream does not hand back its
// real part, nor a real stream pretend to be complex.
func TestReadWrongShape(t *testing.T) {
	iq, err := NewReader(bytes.NewReader(i16(0, 0, 0, 0)), Layout{Format: pcm.I16LE, Rate: 8000, IQ: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := iq.ReadSamples(make([]float64, 2)); err == nil {
		t.Error("a float64 read of an I/Q stream succeeded")
	}

	realRd, err := NewReader(bytes.NewReader(i16(0, 0)), Layout{Format: pcm.I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := realRd.ReadSamples(make([]complex128, 2)); err == nil {
		t.Error("a complex128 read of a real stream succeeded")
	}
	if _, err := realRd.ReadSamples(make([]complex64, 2)); err == nil {
		t.Error("a complex64 read of a real stream succeeded")
	}
}

// TestReadComplexPairs checks that with four interleaved channels a plane is
// an I/Q pair, not a channel.
func TestReadComplexPairs(t *testing.T) {
	// Frame: I0 Q0 I1 Q1.
	raw := i16(0.5, 0.25, -0.5, -0.25)
	l := Layout{Format: pcm.I16LE, Rate: 8000, Channels: 4, IQ: true}
	for pair, want := range []complex128{complex(0.5, 0.25), complex(-0.5, -0.25)} {
		r, err := NewReader(bytes.NewReader(raw), l)
		if err != nil {
			t.Fatal(err)
		}
		if got := drainPlane[complex128](t, r, 2, pair, 2); len(got) != 1 || got[0] != want {
			t.Errorf("pair %d = %v, want %v", pair, got, want)
		}
	}

	r, err := NewReader(bytes.NewReader(raw), l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadSamples(make([]complex128, 2)); err == nil {
		t.Error("ReadSamples read a stream of two I/Q pairs")
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
		opt         Layout
		wantFrames  int64
		wantSeconds float64
	}{
		{"whole file", Layout{Format: pcm.I16LE, Rate: 100}, 100, 1},
		{"stereo halves it", Layout{Format: pcm.I16LE, Rate: 100, Channels: 2}, 50, 0.5},
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

func TestReaderRejectsBadLayout(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  Layout
	}{
		{"no format", Layout{Rate: 8000}},
		{"no rate", Layout{Format: pcm.I16LE}},
		{"invalid format", Layout{Format: pcm.Format(99), Rate: 8000}},
		{"negative channels", Layout{Format: pcm.I16LE, Rate: 8000, Channels: -1}},
		{"an unknown container", Layout{Format: pcm.I16LE, Rate: 8000, Container: 9}},
		{"odd channels for I/Q", Layout{Format: pcm.I16LE, Rate: 8000, IQ: true, Channels: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewReader(bytes.NewReader(make([]byte, 64)), tc.opt); err == nil {
				t.Error("accepted a layout that should be rejected")
			}
		})
	}
}

// TestReadFloat32 covers the width the filters in dsp take, which is what the
// examples need.
func TestReadFloat32(t *testing.T) {
	r, err := NewReader(bytes.NewReader(i16(0.5, -0.5, 0.25)), Layout{Format: pcm.I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 4)
	n, err := r.ReadSamples(dst)
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
	r, err := NewReader(bytes.NewReader(i16(0.5)), Layout{Format: pcm.I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, r, 4)
	for range 3 {
		if n, err := r.ReadSamples(make([]float64, 4)); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("after EOF, ReadSamples = (%d, %v), want (0, EOF)", n, err)
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
		n, err := r.ReadPlanar(planes)
		for c := range out {
			out[c] = append(out[c], planes[c][:n]...)
		}
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("ReadPlanar: %v", err)
		}
		if n == 0 {
			t.Fatal("ReadPlanar returned 0, nil, which would spin")
		}
	}
}

// TestReadPlanarMatchesDecode checks a planar read of every format and channel
// count against pcm.Decode run over each channel's bytes, bit for bit, both
// with every plane present and with one plane at a time and the rest nil.
func TestReadPlanarMatchesDecode(t *testing.T) {
	blocks := []int{1, 2, 3, 7, 64, 299, 300, 301, 4096}
	for _, format := range pcm.Formats() {
		for _, channels := range []int{1, 2, 3, 8} {
			const frames = 300
			width := format.Width()
			raw := make([]byte, frames*channels*width)
			for i := range raw {
				raw[i] = byte(i*31 + 7)
			}
			l := Layout{Format: format, Rate: 8000, Channels: channels, Container: Raw}

			want := make([][]float64, channels)
			for c := range want {
				var one []byte
				for i := range frames {
					at := (i*channels + c) * width
					one = append(one, raw[at:at+width]...)
				}
				want[c] = make([]float64, frames)
				pcm.Decode(want[c], one, format)
			}
			// Arbitrary bytes read as a float format can be NaN, which is
			// never equal to itself.
			same := func(t *testing.T, c int, got []float64) {
				t.Helper()
				if len(got) != frames {
					t.Fatalf("channel %d: %d frames, want %d", c, len(got), frames)
				}
				for i, w := range want[c] {
					if got[i] != w && !(math.IsNaN(got[i]) && math.IsNaN(w)) {
						t.Fatalf("channel %d frame %d = %v, want %v", c, i, got[i], w)
					}
				}
			}

			for _, block := range blocks {
				t.Run(fmt.Sprintf("%v/%dch/block%d", format, channels, block), func(t *testing.T) {
					r, err := NewReader(bytes.NewReader(raw), l)
					if err != nil {
						t.Fatal(err)
					}
					for c, got := range readAllPlanar(t, r, channels, block) {
						same(t, c, got)
					}
					for c := range channels {
						r, err := NewReader(bytes.NewReader(raw), l)
						if err != nil {
							t.Fatal(err)
						}
						same(t, c, drainPlane[float64](t, r, channels, c, block))
					}
				})
			}
		}
	}
}

// TestReadPlanarMono covers a planar read of a one-channel stream, which
// decodes straight into the plane with no split.
func TestReadPlanarMono(t *testing.T) {
	want := []float64{0, 0.5, -0.5, 0.25, -0.125}
	for _, block := range []int{1, 2, 3, 5, 64} {
		r, err := NewReader(bytes.NewReader(i16(want...)), Layout{Format: pcm.I16LE, Rate: 8000})
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

// TestReadPlanarShortestPlaneWins pins the mismatched-length rule, and the half
// of it that makes the rule safe: the frames that did not fit were not consumed
// either, so the next call continues where this one stopped.
func TestReadPlanarShortestPlaneWins(t *testing.T) {
	// Codes 0..15 rather than amplitudes, so each frame says which one it is.
	raw := make([]byte, 32)
	for i := range 16 {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(int16(i)))
	}
	r, err := NewReader(bytes.NewReader(raw), Layout{
		Format: pcm.I16LE, Rate: 8000, Channels: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	long, short := make([]float64, 5), make([]float64, 3)
	n, err := r.ReadPlanar([][]float64{long, short})
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
	n, err = r.ReadPlanar([][]float64{long, short})
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

// TestReadPlanarTruncatedFrame checks that a trailing partial frame is dropped
// rather than half-decoded, as TestReadTruncatedFrame does for one channel.
func TestReadPlanarTruncatedFrame(t *testing.T) {
	raw := append(i16(0.5, -0.5, 0.25, -0.25), 0x00) // two whole frames plus a stray byte
	r, err := NewReader(bytes.NewReader(raw), Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	got := readAllPlanar(t, r, 2, 4)
	if len(got[0]) != 2 || len(got[1]) != 2 {
		t.Fatalf("got %d and %d frames, want 2 each", len(got[0]), len(got[1]))
	}
}

// TestReadPlanarNilPlanes checks the rules for a nil plane: its channel is
// passed over, it does not count toward the shortest plane, and a read with
// nothing but nil planes is an error, not an empty read.
func TestReadPlanarNilPlanes(t *testing.T) {
	// Three channels, four frames; each value names its channel and frame.
	raw := i16(0.00, 0.10, 0.20, 0.01, 0.11, 0.21, 0.02, 0.12, 0.22, 0.03, 0.13, 0.23)
	want := func(c, i int) float64 { return roundTrip16(float64(c)/10 + float64(i)/100) }
	open := func() *Reader {
		r, err := NewReader(bytes.NewReader(raw), Layout{Format: pcm.I16LE, Rate: 8000, Channels: 3})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	r := open()
	first, last := make([]float64, 3), make([]float32, 3)
	n, err := r.ReadPlanar([][]float64{first, nil, nil})
	if err != nil || n != 3 {
		t.Fatalf("ReadPlanar = %d, %v, want 3 frames", n, err)
	}
	for i := range n {
		if first[i] != want(0, i) {
			t.Errorf("channel 0 frame %d = %v, want %v", i, first[i], want(0, i))
		}
	}
	// The skipped channels were consumed with the frame: the next read, at
	// another width and of another channel, continues at frame 3.
	n, err = r.ReadPlanar([][]float32{nil, nil, last})
	if err != nil || n != 1 {
		t.Fatalf("second ReadPlanar = %d, %v, want the 1 frame left", n, err)
	}
	if last[0] != float32(want(2, 3)) {
		t.Errorf("channel 2 frame 3 = %v, want %v", last[0], want(2, 3))
	}

	r = open()
	if n, err := r.ReadPlanar([][]float64{nil, nil, nil}); err == nil {
		t.Errorf("a read into nothing but nil planes returned %d frames and no error", n)
	}
	if got := readAllPlanar(t, r, 3, 8); len(got[0]) != 4 {
		t.Errorf("%d frames left after the refused read, want all 4", len(got[0]))
	}
}

// TestReadPlanarRejectsWrongShape checks the structural mistakes, which are
// loud, as against a mismatched plane length, which is not.
func TestReadPlanarRejectsWrongShape(t *testing.T) {
	planes := func(n, size int) [][]float64 {
		p := make([][]float64, n)
		for i := range p {
			p[i] = make([]float64, size)
		}
		return p
	}
	for _, tt := range []struct {
		name   string
		opt    Layout
		planes int
	}{
		{"too few planes", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2}, 1},
		{"too many planes", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2}, 3},
		{"no planes", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2}, 0},
		{"real planes for an I/Q stream", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2, IQ: true}, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(i16(0, 0, 0, 0)), tt.opt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.ReadPlanar(planes(tt.planes, 4)); err == nil {
				t.Error("float64 planes: want an error")
			}
			f32 := make([][]float32, tt.planes)
			for i := range f32 {
				f32[i] = make([]float32, 4)
			}
			if _, err := r.ReadPlanar(f32); err == nil {
				t.Error("float32 planes: want an error")
			}
		})
	}
}

// TestReadPlanarEOFIsSticky makes sure a drained Reader keeps saying so rather
// than blocking or spinning, as TestReadEOFIsSticky does for one channel.
func TestReadPlanarEOFIsSticky(t *testing.T) {
	r, err := NewReader(bytes.NewReader(i16(0.5, -0.5)), Layout{
		Format: pcm.I16LE, Rate: 8000, Channels: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	readAllPlanar(t, r, 2, 4)
	for range 3 {
		planes := [][]float64{make([]float64, 4), make([]float64, 4)}
		if n, err := r.ReadPlanar(planes); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("after EOF, ReadPlanar = (%d, %v), want (0, EOF)", n, err)
		}
	}
}

// BenchmarkReadPlanar measures a planar read of a whole stream, with every
// plane present and with one plane and the rest nil. SetBytes is the size of
// the stream in both, so the figures compare directly. Raw keeps sniffWAV's
// bufio wrap out of it.
func BenchmarkReadPlanar(b *testing.B) {
	const frames = 1 << 16
	for _, channels := range []int{2, 4, 8} {
		raw := make([]byte, frames*channels*2)
		for i := range raw {
			raw[i] = byte(i*31 + 7)
		}
		l := Layout{Format: pcm.I16LE, Rate: 8000, Channels: channels, Container: Raw}

		all := make([][]float64, channels)
		for c := range all {
			all[c] = make([]float64, 4096)
		}
		one := make([][]float64, channels)
		one[channels-1] = make([]float64, 4096)

		for _, arm := range []struct {
			name   string
			planes [][]float64
		}{{"all", all}, {"one", one}} {
			b.Run(fmt.Sprintf("%dch/%s", channels, arm.name), func(b *testing.B) {
				b.SetBytes(int64(len(raw)))
				b.ReportAllocs()
				for b.Loop() {
					r, err := NewReader(bytes.NewReader(raw), l)
					if err != nil {
						b.Fatal(err)
					}
					for {
						if _, err := r.ReadPlanar(arm.planes); err != nil {
							break
						}
					}
				}
			})
		}
	}
}

// TestFloatOverFullScaleNote checks the Note for a float sample outside full
// scale. It can appear only once the sample has been read, so Info is checked
// before and after.
func TestFloatOverFullScaleNote(t *testing.T) {
	raw := make([]byte, 0, 4*4)
	for _, v := range []float32{0.5, -0.25, 2.0, 0.1} {
		raw = binary.LittleEndian.AppendUint32(raw, math.Float32bits(v))
	}

	r, err := NewReader(io.NopCloser(bytes.NewReader(raw)), Layout{
		Format: pcm.F32LE, Rate: 8000, Channels: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := noteAbout(r.Info().Notes, "full scale"); got != "" {
		t.Fatalf("the note appeared before anything was read: %q", got)
	}

	dst := make([]float64, 4)
	if n, err := r.ReadSamples(dst); err != nil || n != 4 {
		t.Fatalf("ReadSamples = %d, %v", n, err)
	}
	if got := noteAbout(r.Info().Notes, "full scale"); got == "" {
		t.Errorf("no note about full scale in %q", r.Info().Notes)
	}

	// An integer format cannot produce one, since every code divides into
	// range by construction.
	ri, err := NewReader(io.NopCloser(bytes.NewReader([]byte{0x00, 0x80, 0xff, 0x7f})), Layout{
		Format: pcm.I16LE, Rate: 8000, Channels: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ri.ReadSamples(make([]float64, 2)); err != nil {
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
		r, err := NewReader(io.NopCloser(stalledReader{}), Layout{Format: pcm.I16LE, Rate: 8000, Channels: 1})
		if err != nil {
			done <- err
			return
		}
		_, err = r.ReadSamples(make([]float64, 16))
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

// BenchmarkRead measures a whole-stream read at each width, mono I16LE and
// rtl-sdr-style U8 I/Q among them. SetBytes is the size of the stream.
func BenchmarkRead(b *testing.B) {
	b.Run("i16le/float32", func(b *testing.B) { benchRead[float32](b, pcm.I16LE, false) })
	b.Run("i16le/float64", func(b *testing.B) { benchRead[float64](b, pcm.I16LE, false) })
	b.Run("cu8/complex64", func(b *testing.B) { benchRead[complex64](b, pcm.U8, true) })
	b.Run("cu8/complex128", func(b *testing.B) { benchRead[complex128](b, pcm.U8, true) })
	b.Run("ci16le/complex64", func(b *testing.B) { benchRead[complex64](b, pcm.I16LE, true) })
}

func benchRead[T dsp.Sample](b *testing.B, f pcm.Format, iq bool) {
	const frames = 1 << 16
	channels := 1
	if iq {
		channels = 2
	}
	raw := make([]byte, frames*channels*f.Width())
	for i := range raw {
		raw[i] = byte(i*31 + 7)
	}
	opt := Layout{Format: f, Rate: 8000, IQ: iq, Container: Raw}
	buf := make([]T, 4096)
	b.SetBytes(int64(len(raw)))
	b.ReportAllocs()
	for b.Loop() {
		r, err := NewReader(bytes.NewReader(raw), opt)
		if err != nil {
			b.Fatal(err)
		}
		for {
			if _, err := r.ReadSamples(buf); err != nil {
				break
			}
		}
	}
}

// TestReadIntoNothing checks that a read into an empty slice takes nothing
// from the stream and is not mistaken for its end, whether or not the stream
// says how long it is.
func TestReadIntoNothing(t *testing.T) {
	payload := i16(0.5, -0.5, 0.25)
	for _, size := range []uint32{uint32(len(payload)), math.MaxUint32} {
		r, err := NewReader(bytes.NewReader(wav(pcmFmt(wavPCM, 1, 8000, 16), size, payload)), Layout{})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := r.ReadSamples([]float32(nil)); n != 0 || err != nil {
			t.Errorf("size %#x: an empty read at the start = (%d, %v), want (0, nil)", size, n, err)
		}
		if n, err := r.ReadPlanar([][]float64{{}}); n != 0 || err != nil {
			t.Errorf("size %#x: an empty planar read = (%d, %v), want (0, nil)", size, n, err)
		}
		if got := drain[float64](t, r, 8); len(got) != 3 {
			t.Errorf("size %#x: %d frames after the empty reads, want 3", size, len(got))
		}
	}
}
