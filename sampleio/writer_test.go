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

	"samuelks.com/dsp"
	"samuelks.com/dsp/pcm"
)

// seekBuffer is an in-memory io.WriteSeeker, the way a file looks to a Writer.
type seekBuffer struct {
	b   []byte
	off int
}

func (s *seekBuffer) Write(p []byte) (int, error) {
	if need := s.off + len(p); need > len(s.b) {
		s.b = append(s.b, make([]byte, need-len(s.b))...)
	}
	copy(s.b[s.off:], p)
	s.off += len(p)
	return len(p), nil
}

func (s *seekBuffer) Seek(off int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		off += int64(s.off)
	case io.SeekEnd:
		off += int64(len(s.b))
	}
	if off < 0 {
		return 0, errors.New("seek before the start")
	}
	s.off = int(off)
	return off, nil
}

// testSignal is frames frames of channels channels: a ramp across full scale
// with a few samples past it, which an integer format clips.
func testSignal(channels, frames int) [][]float64 {
	planes := make([][]float64, channels)
	for c := range planes {
		p := make([]float64, frames)
		for i := range p {
			p[i] = math.Sin(float64(i*(c+3))*0.37) * 1.05
		}
		planes[c] = p
	}
	return planes
}

// encoded is what x reads back as through format f: the encode then the
// decode, at x's own width.
func encoded[T dsp.Sample](x []T, f pcm.Format) []T {
	b := make([]byte, len(x)*f.Width()*elemsOf[T]())
	pcm.Encode(b, x, f)
	out := make([]T, len(x))
	pcm.Decode(out, b, f)
	return out
}

func elemsOf[T dsp.Sample]() int {
	if isComplex[T]() {
		return 2
	}
	return 1
}

func narrowPlanes[T dsp.Float](x [][]float64) [][]T {
	out := make([][]T, len(x))
	for c, p := range x {
		out[c] = make([]T, len(p))
		for i, v := range p {
			out[c][i] = T(v)
		}
	}
	return out
}

// TestWriteReadRoundTrip writes every format, channel count and container at
// both widths, to a destination that can seek and one that cannot, and reads
// it back: the samples must be exactly the format's encoding of what went in,
// and the Info the Reader finds must be the Layout the Writer was given.
func TestWriteReadRoundTrip(t *testing.T) {
	wavFormats := []pcm.Format{pcm.U8, pcm.I16LE, pcm.I24LE, pcm.I32LE, pcm.F32LE, pcm.F64LE, pcm.ALaw, pcm.MuLaw}
	for _, container := range []Container{WAV, Raw} {
		formats := wavFormats
		if container == Raw {
			formats = pcm.Formats()
		}
		for _, f := range formats {
			for _, channels := range []int{1, 2, 3} {
				for _, seek := range []bool{true, false} {
					name := fmt.Sprintf("%v/%v/%dch/seek=%v", container, f, channels, seek)
					t.Run(name+"/float64", func(t *testing.T) {
						roundTrip[float64](t, Layout{Format: f, Rate: 8000, Channels: channels, Container: container}, seek)
					})
					t.Run(name+"/float32", func(t *testing.T) {
						roundTrip[float32](t, Layout{Format: f, Rate: 8000, Channels: channels, Container: container}, seek)
					})
				}
			}
		}
	}
}

func roundTrip[T dsp.Float](t *testing.T, l Layout, seek bool) {
	t.Helper()
	const frames = 1001 // odd, so a U8 mono WAV needs its pad byte
	in := narrowPlanes[T](testSignal(l.Channels, frames))
	raw := writeAll(t, l, seek, func(w *Writer) (int, error) {
		if l.Channels == 1 {
			return w.WriteSamples(in[0][:400])
		}
		return w.WritePlanar(in)
	}, func(w *Writer) (int, error) {
		if l.Channels == 1 {
			return w.WriteSamples(in[0][400:])
		}
		return 0, nil
	})

	r, err := NewReader(bytes.NewReader(raw), l)
	if err != nil {
		t.Fatal(err)
	}
	// An extensible header is given a mask when the layout has none, which
	// TestWAVChannelMask covers.
	back := r.Info().Layout
	back.ChannelMask = l.ChannelMask
	if back != l {
		t.Errorf("read back %+v, wrote %+v", back, l)
	}
	got := drainPlanar[T](t, r, l.Channels, 256)
	for c := range in {
		want := encoded(in[c], l.Format)
		if len(got[c]) != len(want) {
			t.Fatalf("channel %d: %d frames, want %d", c, len(got[c]), len(want))
		}
		for i := range want {
			if got[c][i] != want[i] {
				t.Fatalf("channel %d frame %d = %v, want %v", c, i, got[c][i], want[i])
			}
		}
	}
	if l.Container == WAV && seek && len(raw)%2 != 0 {
		t.Errorf("a WAV of %d bytes; chunks are word-aligned", len(raw))
	}
}

// writeAll writes through a Writer to memory with each write in turn, closes
// it, and returns the bytes.
func writeAll(t *testing.T, opt Layout, seek bool, writes ...func(*Writer) (int, error)) []byte {
	t.Helper()
	var dst io.Writer
	sb := &seekBuffer{}
	var pipe bytes.Buffer
	if seek {
		dst = sb
	} else {
		dst = &pipe
	}
	w, err := NewWriter(dst, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, write := range writes {
		if _, err := write(w); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if seek {
		if sb.off != len(sb.b) {
			t.Errorf("Close left the destination at %d of %d", sb.off, len(sb.b))
		}
		return sb.b
	}
	return pipe.Bytes()
}

// TestWriteIQ writes an I/Q stream at both complex widths, through
// WriteSamples and WritePlanar, and reads it back.
func TestWriteIQ(t *testing.T) {
	x := make([]complex128, 777)
	for i := range x {
		x[i] = complex(math.Cos(float64(i)*0.1), math.Sin(float64(i)*0.1))
	}
	for _, f := range []pcm.Format{pcm.U8, pcm.I16LE, pcm.F32LE} {
		l := Layout{Format: f, Rate: 2_400_000, IQ: true, Container: Raw}
		raw := writeAll(t, l, false, func(w *Writer) (int, error) {
			return w.WriteSamples(x[:300])
		}, func(w *Writer) (int, error) {
			return w.WritePlanar([][]complex128{x[300:]})
		})
		r, err := NewReader(bytes.NewReader(raw), l)
		if err != nil {
			t.Fatal(err)
		}
		narrowsTo(t, f.String(), encoded(x, f), drain[complex128](t, r, 64), func(v complex128) complex128 { return v })

		x64 := make([]complex64, len(x))
		for i, v := range x {
			x64[i] = complex64(v)
		}
		l.Container = WAV
		raw = writeAll(t, l, true, func(w *Writer) (int, error) { return w.WriteSamples(x64) })
		r, err = NewReader(bytes.NewReader(raw), Layout{IQ: true})
		if err != nil {
			t.Fatal(err)
		}
		if info := r.Info(); info.Channels != 2 || !info.IQ {
			t.Errorf("%v: read back %+v", f, info.Layout)
		}
		narrowsTo(t, f.String()+" complex64", encoded(x64, f), drain[complex64](t, r, 64), func(v complex64) complex64 { return v })
	}
}

// TestWriterReproducesFixtures reads each fixture and writes it again with
// the Layout the Reader reported: the Reader must find the same Layout, frame
// count and samples in the copy.
func TestWriterReproducesFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.wav"))
	if err != nil || len(files) == 0 {
		t.Skip("no fixtures alongside the test binary")
	}
	dir := t.TempDir()
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			r, err := Open(path, Layout{})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
			info := r.Info()
			want := drainPlanar[float64](t, r, info.Channels, 50)

			out := filepath.Join(dir, filepath.Base(path))
			w, err := Create(out, info.Layout)
			if err != nil {
				t.Fatal(err)
			}
			if info.Channels == 1 {
				_, err = w.WriteSamples(want[0])
			} else {
				_, err = w.WritePlanar(want)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}

			r2, err := Open(out, Layout{})
			if err != nil {
				t.Fatal(err)
			}
			defer r2.Close() //nolint:errcheck // as above
			info2 := r2.Info()
			if info2.Layout != info.Layout || info2.Frames != info.Frames || info2.Bits != info.Bits {
				t.Errorf("copy reads as %+v, original %+v", info2, info)
			}
			got := drainPlanar[float64](t, r2, info.Channels, 50)
			for c := range want {
				narrowsTo(t, fmt.Sprint("channel ", c), want[c], got[c], func(v float64) float64 { return v })
			}
		})
	}
}

// TestWAVHeaderChoice checks the format tag and the fields around it for each
// kind of header the Writer emits.
func TestWAVHeaderChoice(t *testing.T) {
	for _, tc := range []struct {
		f        pcm.Format
		channels int
		tag      uint16
		subtype  uint16 // for an extensible header
		mask     uint32
		fact     bool
	}{
		{pcm.U8, 1, wavPCM, 0, 0, false},
		{pcm.I16LE, 2, wavPCM, 0, 0, false},
		{pcm.I16LE, 3, wavExtensible, wavPCM, 0, false},
		{pcm.I24LE, 1, wavExtensible, wavPCM, maskMono, false},
		{pcm.I32LE, 2, wavExtensible, wavPCM, maskStereo, false},
		{pcm.F32LE, 1, wavFloat, 0, 0, true},
		{pcm.F64LE, 2, wavFloat, 0, 0, true},
		{pcm.F32LE, 4, wavExtensible, wavFloat, 0, true},
		{pcm.ALaw, 1, wavALaw, 0, 0, true},
		{pcm.MuLaw, 2, wavMuLaw, 0, 0, true},
		{pcm.MuLaw, 6, wavExtensible, wavMuLaw, 0, true},
	} {
		t.Run(fmt.Sprintf("%v/%dch", tc.f, tc.channels), func(t *testing.T) {
			hdr, l, err := wavHeaderBytes(tc.f, tc.channels, 8000, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			if l.junk != -1 {
				t.Error("a JUNK chunk without asking for one")
			}
			le := binary.LittleEndian
			fmtAt := bytes.Index(hdr, []byte("fmt "))
			if tag := le.Uint16(hdr[fmtAt+8:]); tag != tc.tag {
				t.Errorf("tag %#04x, want %#04x", tag, tc.tag)
			}
			if tc.tag == wavExtensible {
				if sub := le.Uint16(hdr[fmtAt+8+24:]); sub != tc.subtype {
					t.Errorf("subtype %#04x, want %#04x", sub, tc.subtype)
				}
				if mask := le.Uint32(hdr[fmtAt+8+20:]); mask != tc.mask {
					t.Errorf("mask %#x, want %#x", mask, tc.mask)
				}
			}
			if got := bytes.Contains(hdr, []byte("fact")); got != tc.fact {
				t.Errorf("fact chunk %v, want %v", got, tc.fact)
			}
		})
	}
}

// TestWriterSizes checks the sizes Close patches into a file that can seek,
// and the placeholders a pipe gets.
func TestWriterSizes(t *testing.T) {
	x := make([]float32, 101)
	le := binary.LittleEndian
	opt := Layout{Format: pcm.F32LE, Rate: 8000}
	file := writeAll(t, opt, true, func(w *Writer) (int, error) { return w.WriteSamples(x) })
	pipe := writeAll(t, opt, false, func(w *Writer) (int, error) { return w.WriteSamples(x) })

	if got := le.Uint32(file[4:]); int(got) != len(file)-8 {
		t.Errorf("RIFF size %d, want %d", got, len(file)-8)
	}
	if !bytes.Contains(file, []byte("JUNK")) {
		t.Error("a seekable file with no JUNK chunk to grow into ds64")
	}
	dataAt := bytes.Index(file, []byte("data"))
	if got := le.Uint32(file[dataAt+4:]); got != 4*101 {
		t.Errorf("data size %d, want %d", got, 4*101)
	}
	factAt := bytes.Index(file, []byte("fact"))
	if got := le.Uint32(file[factAt+8:]); got != 101 {
		t.Errorf("fact count %d, want 101", got)
	}

	if bytes.Contains(pipe, []byte("JUNK")) {
		t.Error("a pipe got a JUNK chunk it can never patch")
	}
	if got := le.Uint32(pipe[4:]); got != math.MaxUint32 {
		t.Errorf("pipe RIFF size %#x, want the placeholder", got)
	}
	dataAt = bytes.Index(pipe, []byte("data"))
	if got := le.Uint32(pipe[dataAt+4:]); got != math.MaxUint32 {
		t.Errorf("pipe data size %#x, want the placeholder", got)
	}
	factAt = bytes.Index(pipe, []byte("fact"))
	if got := le.Uint32(pipe[factAt+8:]); got != 0 {
		t.Errorf("pipe fact count %d, want 0", got)
	}

	// A file reports its length; the pipe form reads to the end.
	dir := t.TempDir()
	for name, b := range map[string][]byte{"file.wav": file, "pipe.wav": pipe} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		r, err := Open(path, Layout{})
		if err != nil {
			t.Fatal(err)
		}
		if got := len(drain[float32](t, r, 64)); got != 101 {
			t.Errorf("%s: read %d frames, want 101", name, got)
		}
		if name == "file.wav" && r.Info().Frames != 101 {
			t.Errorf("%s: Frames = %d, want 101", name, r.Info().Frames)
		}
		_ = r.Close()
	}
}

// TestWriterRF64 lowers the size limit so a small file crosses it, and checks
// that Close turns the reserved JUNK into ds64 and the Reader follows it.
func TestWriterRF64(t *testing.T) {
	defer func(v int64) { rf64Limit = v }(rf64Limit)
	rf64Limit = 200

	x := make([]float64, 300)
	for i := range x {
		x[i] = float64(i%200-100) / 128
	}
	opt := Layout{Format: pcm.I16LE, Rate: 8000}
	b := writeAll(t, opt, true, func(w *Writer) (int, error) { return w.WriteSamples(x) })
	if string(b[:4]) != "RF64" || !bytes.Contains(b, []byte("ds64")) || bytes.Contains(b, []byte("JUNK")) {
		t.Fatalf("header %q is not RF64 with a ds64 chunk", b[:48])
	}

	path := filepath.Join(t.TempDir(), "rf64.wav")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path, Layout{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	if r.Info().Frames != 300 {
		t.Errorf("Frames = %d, want 300", r.Info().Frames)
	}
	narrowsTo(t, "rf64", encoded(x, pcm.I16LE), drain[float64](t, r, 64), func(v float64) float64 { return v })
}

// TestWriterClipNote checks that an integer format notes a clipped sample,
// once, and a float format, which holds any value, never does.
func TestWriterClipNote(t *testing.T) {
	for _, tc := range []struct {
		f    pcm.Format
		x    []float32
		note bool
	}{
		{pcm.I16LE, []float32{0.5, -1, 0.99}, false},
		{pcm.I16LE, []float32{0.5, 1.5, -0.2}, true},
		{pcm.U8, []float32{-1.01}, true},
		{pcm.F32LE, []float32{0.5, 2, -3}, false},
		// A-law's top step reaches full scale. Mu-law's stops short of it, and
		// a sample past its edge has only the top level left to take.
		{pcm.ALaw, []float32{1, -1}, false},
		{pcm.ALaw, []float32{1.01}, true},
		{pcm.MuLaw, []float32{0.995, -0.995}, false},
		{pcm.MuLaw, []float32{0.997}, true},
	} {
		w, err := NewWriter(io.Discard, Layout{Format: tc.f, Rate: 8000})
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if _, err := w.WriteSamples(tc.x); err != nil {
				t.Fatal(err)
			}
		}
		notes := 0
		for _, n := range w.Info().Notes {
			if strings.Contains(n, "clipped") {
				notes++
			}
		}
		if want := map[bool]int{false: 0, true: 1}[tc.note]; notes != want {
			t.Errorf("%v %v: %d clip notes, want %d", tc.f, tc.x, notes, want)
		}
	}

	// A complex write is checked too.
	w, err := NewWriter(io.Discard, Layout{Format: pcm.U8, IQ: true, Container: Raw})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteSamples([]complex64{complex(0, -1.5)}); err != nil {
		t.Fatal(err)
	}
	if len(w.Info().Notes) != 1 {
		t.Errorf("complex clip: notes %q", w.Info().Notes)
	}
}

func TestWriterRejectsBadLayout(t *testing.T) {
	for _, tc := range []struct {
		name string
		l    Layout
	}{
		{"no format", Layout{Rate: 8000}},
		{"I8 in a WAV", Layout{Format: pcm.I8, Rate: 8000}},
		{"big-endian in a WAV", Layout{Format: pcm.I16BE, Rate: 8000}},
		{"no rate for a WAV", Layout{Format: pcm.I16LE}},
		{"fractional WAV rate", Layout{Format: pcm.I16LE, Rate: 8000.5}},
		{"WAV rate past 2^32", Layout{Format: pcm.I16LE, Rate: 1 << 32}},
		{"negative rate", Layout{Format: pcm.I16LE, Rate: -1, Container: Raw}},
		{"NaN rate", Layout{Format: pcm.I16LE, Rate: math.NaN(), Container: Raw}},
		{"IQ with 4 channels", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 4, IQ: true}},
		{"IQ with 1 channel", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 1, IQ: true}},
		{"negative channels", Layout{Format: pcm.I16LE, Rate: 8000, Channels: -1}},
		{"too many WAV channels", Layout{Format: pcm.I16LE, Rate: 8000, Channels: 1 << 16}},
		{"unknown container", Layout{Format: pcm.I16LE, Rate: 8000, Container: 9}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewWriter(io.Discard, tc.l); err == nil {
				t.Error("accepted")
			}
		})
	}

	// A bad option leaves an existing file as it was.
	path := filepath.Join(t.TempDir(), "keep.wav")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(path, Layout{Format: pcm.I8, Rate: 8000}); err == nil {
		t.Fatal("Create accepted I8 in a WAV")
	}
	if b, _ := os.ReadFile(path); string(b) != "keep" {
		t.Errorf("a refused Create changed the file to %q", b)
	}
}

func TestWriterMethodErrors(t *testing.T) {
	stereo := func() *Writer {
		w, err := NewWriter(io.Discard, Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	iq := func() *Writer {
		w, err := NewWriter(io.Discard, Layout{Format: pcm.I16LE, Rate: 8000, IQ: true})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	for _, tc := range []struct {
		name  string
		write func() (int, error)
	}{
		{"one slice for two channels", func() (int, error) { return stereo().WriteSamples(make([]float64, 4)) }},
		{"complex to a real stream", func() (int, error) { return stereo().WritePlanar(make([][]complex64, 2)) }},
		{"real to an I/Q stream", func() (int, error) { return iq().WriteSamples(make([]float32, 4)) }},
		{"real planes to an I/Q stream", func() (int, error) { return iq().WritePlanar(make([][]float64, 2)) }},
		{"too few planes", func() (int, error) { return stereo().WritePlanar(make([][]float64, 1)) }},
		{"two planes for one pair", func() (int, error) { return iq().WritePlanar(make([][]complex128, 2)) }},
		{"no planes", func() (int, error) { return stereo().WritePlanar([][]float64(nil)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if n, err := tc.write(); err == nil || n != 0 {
				t.Errorf("(%d, %v), want an error and nothing written", n, err)
			}
		})
	}

	// Unequal planes write nothing at all.
	w := stereo()
	if _, err := w.WritePlanar([][]float64{make([]float64, 5), make([]float64, 4)}); err == nil {
		t.Error("unequal planes accepted")
	}
	if w.Info().Frames != 0 {
		t.Errorf("unequal planes wrote %d frames", w.Info().Frames)
	}

	// Everything after Close fails.
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WritePlanar([][]float64{{0}, {0}}); !errors.Is(err, errClosed) {
		t.Errorf("write after Close: %v", err)
	}
	if err := w.Flush(); !errors.Is(err, errClosed) {
		t.Errorf("Flush after Close: %v", err)
	}
	if err := w.Close(); !errors.Is(err, errClosed) {
		t.Errorf("second Close: %v", err)
	}
}

// failAfter accepts n bytes and then fails every write.
type failAfter struct{ n int }

var errDisk = errors.New("disk full")

func (f *failAfter) Write(p []byte) (int, error) {
	if len(p) > f.n {
		m := f.n
		f.n = 0
		return m, errDisk
	}
	f.n -= len(p)
	return len(p), nil
}

// TestWriterStickyError checks that a destination's error is returned by the
// write that met it and by every call after.
func TestWriterStickyError(t *testing.T) {
	w, err := NewWriter(&failAfter{n: 1000}, Layout{Format: pcm.I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	x := make([]float64, 1<<17) // past one buffer, so a write reaches the destination
	n, err := w.WriteSamples(x)
	if !errors.Is(err, errDisk) || n >= len(x) {
		t.Fatalf("(%d, %v), want a short count and the destination's error", n, err)
	}
	if _, err := w.WriteSamples(x[:1]); !errors.Is(err, errDisk) {
		t.Errorf("next write: %v", err)
	}
	if err := w.Flush(); !errors.Is(err, errDisk) {
		t.Errorf("Flush: %v", err)
	}
	if err := w.Close(); !errors.Is(err, errDisk) {
		t.Errorf("Close: %v", err)
	}
}

// TestWriterFlush checks that Flush hands a live consumer everything so far.
func TestWriterFlush(t *testing.T) {
	var pipe bytes.Buffer
	w, err := NewWriter(&pipe, Layout{Format: pcm.U8, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteSamples([]float32{0, 0.5, -0.5}); err != nil {
		t.Fatal(err)
	}
	if pipe.Len() != 0 {
		t.Fatalf("%d bytes reached the pipe before Flush", pipe.Len())
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader(bytes.NewReader(pipe.Bytes()), Layout{})
	if err != nil {
		t.Fatal(err)
	}
	if got := drain[float32](t, r, 8); len(got) != 3 || got[1] != 0.5 {
		t.Errorf("a consumer read %v after Flush", got)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestCreateStdout checks that "-" writes to standard output, which Close
// leaves open. Standard output is a temporary file here, so the sizes are
// patched as for any file that can seek.
func TestCreateStdout(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // a temporary file the test owns
	saved := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = saved }()

	w, err := Create("-", Layout{Format: pcm.I16LE, Rate: 8000})
	os.Stdout = saved
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteSamples([]float64{0.25, -0.25}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Stat(); err != nil {
		t.Fatalf("standard output was closed: %v", err)
	}
	r, err := Open(f.Name(), Layout{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	if r.Info().Frames != 2 {
		t.Errorf("Frames = %d, want 2", r.Info().Frames)
	}
}

// FuzzWriteRead writes arbitrary samples in an arbitrary layout and reads them
// back: they must come back as the format's encoding of what went in.
func FuzzWriteRead(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7}, uint8(1), uint8(0), true)
	f.Add([]byte("\x7f\xff\x80\x00\x01\x00\xff\x7f"), uint8(3), uint8(1), false)
	f.Add(make([]byte, 33), uint8(5), uint8(2), true)
	f.Add([]byte("\x00\x00\x80\x3f\x00\x00\x00\xbf\x00\x00\x80\x39"), uint8(7), uint8(0), true)
	f.Fuzz(func(t *testing.T, in []byte, format, channels uint8, seek bool) {
		formats := []pcm.Format{pcm.U8, pcm.I16LE, pcm.I24LE, pcm.I32LE, pcm.F32LE, pcm.F64LE, pcm.ALaw, pcm.MuLaw}
		fm := formats[int(format)%len(formats)]
		ch := 1 + int(channels)%3
		frames := len(in) / 4 / ch
		planes := make([][]float32, ch)
		for c := range planes {
			planes[c] = make([]float32, frames)
			for i := range planes[c] {
				off := (i*ch + c) * 4
				planes[c][i] = math.Float32frombits(binary.LittleEndian.Uint32(in[off:]))
			}
		}
		l := Layout{Format: fm, Rate: 8000, Channels: ch, Container: WAV}
		raw := writeAll(t, l, seek, func(w *Writer) (int, error) { return w.WritePlanar(planes) })
		r, err := NewReader(bytes.NewReader(raw), Layout{})
		if err != nil {
			t.Fatal(err)
		}
		got := drainPlanar[float32](t, r, ch, 17)
		for c := range planes {
			narrowsTo(t, fmt.Sprint("channel ", c), encoded(planes[c], fm), got[c], func(v float32) float32 { return v })
		}
	})
}

// TestG711Transcode reads every G.711 code out of a WAV, writes the samples
// as 16-bit PCM, reads that and writes it back as G.711. Decoding is exact and
// 16 bits hold every level, so the codes come back as they were, but for
// mu-law's negative zero.
func TestG711Transcode(t *testing.T) {
	codes := make([]float32, 256)
	for _, f := range []pcm.Format{pcm.ALaw, pcm.MuLaw} {
		l := Layout{Format: f, Rate: 8000, Channels: 1, Container: WAV}
		all := make([]byte, 256)
		for i := range all {
			all[i] = byte(i)
		}
		pcm.Decode(codes, all, f)

		file := writeAll(t, l, true, func(w *Writer) (int, error) { return w.WriteSamples(codes) })
		at := bytes.Index(file, []byte("data")) + 8
		want := bytes.Clone(all)
		if f == pcm.MuLaw {
			want[0x7f] = 0xff
		}
		if !bytes.Equal(file[at:at+256], want) {
			t.Fatalf("%v: the codes were written as % x", f, file[at:at+256])
		}

		r, err := NewReader(bytes.NewReader(file), Layout{})
		if err != nil {
			t.Fatal(err)
		}
		linear := l
		linear.Format = pcm.I16LE
		wide := writeAll(t, linear, true, func(w *Writer) (int, error) {
			return w.WriteSamples(drain[float32](t, r, 100))
		})

		r, err = NewReader(bytes.NewReader(wide), Layout{})
		if err != nil {
			t.Fatal(err)
		}
		back := writeAll(t, l, true, func(w *Writer) (int, error) {
			return w.WriteSamples(drain[float32](t, r, 100))
		})
		if !bytes.Equal(back, file) {
			t.Errorf("%v: the file did not survive a trip through 16-bit PCM", f)
		}
	}
}

// TestWriterAtOffset writes a WAV into a destination that already holds
// something: Close patches the sizes relative to where the WAV began and
// leaves what came before it alone.
func TestWriterAtOffset(t *testing.T) {
	sb := &seekBuffer{b: []byte("xyz"), off: 3}
	w, err := NewWriter(sb, Layout{Format: pcm.MuLaw, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	x := []float32{0.5, -0.25, 0}
	if _, err := w.WriteSamples(x); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if string(sb.b[:3]) != "xyz" {
		t.Errorf("the prefix was overwritten: %q", sb.b[:3])
	}
	if got := binary.LittleEndian.Uint32(sb.b[3+4:]); int(got) != len(sb.b)-3-8 {
		t.Errorf("RIFF size %d, want %d", got, len(sb.b)-3-8)
	}
	r, err := NewReader(bytes.NewReader(sb.b[3:]), Layout{})
	if err != nil {
		t.Fatal(err)
	}
	got := drain[float32](t, r, 8)
	if want := encoded(x, pcm.MuLaw); !slices.Equal(got, want) {
		t.Errorf("read back %v, want %v", got, want)
	}
}
