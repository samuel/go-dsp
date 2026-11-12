package sampleio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"testing"

	"samuelks.com/dsp"
	"samuelks.com/dsp/pcm"
)

// drain reads a Reader to the end in blocks of block samples.
func drain[T dsp.Sample](t *testing.T, r *Reader, block int) []T {
	t.Helper()
	var out []T
	buf := make([]T, block)
	for {
		n, err := r.ReadSamples(buf)
		out = append(out, buf[:n]...)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("ReadSamples: %v", err)
		}
	}
}

// drainPlanar reads a Reader to the end into planes planes of block frames.
func drainPlanar[T dsp.Sample](t *testing.T, r *Reader, planes, block int) [][]T {
	t.Helper()
	out := make([][]T, planes)
	buf := make([][]T, planes)
	for c := range buf {
		buf[c] = make([]T, block)
	}
	for {
		n, err := r.ReadPlanar(buf)
		for c := range out {
			out[c] = append(out[c], buf[c][:n]...)
		}
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("ReadPlanar: %v", err)
		}
	}
}

// narrowsTo reports whether every element of got is the one of wide converted
// to got's width, with NaN matching NaN.
func narrowsTo[W, N dsp.Sample](t *testing.T, what string, wide []W, got []N, conv func(W) N) {
	t.Helper()
	if len(got) != len(wide) {
		t.Fatalf("%s: %d samples, want %d", what, len(got), len(wide))
	}
	for i := range wide {
		want := conv(wide[i])
		if got[i] != want && (!isNaN(got[i]) || !isNaN(want)) {
			t.Fatalf("%s: sample %d = %v, want %v", what, i, got[i], want)
		}
	}
}

func isNaN[T dsp.Sample](v T) bool {
	switch x := any(v).(type) {
	case float32:
		return x != x
	case float64:
		return x != x
	case complex64:
		return real(x) != real(x) || imag(x) != imag(x)
	case complex128:
		return math.IsNaN(real(x)) || math.IsNaN(imag(x))
	}
	return false
}

func f64to32(v float64) float32       { return float32(v) }
func c128to64(v complex128) complex64 { return complex64(v) }

// TestFixturesAtEveryWidth reads every fixture each way a Reader can hand it
// over -- each channel, the mixdown, every plane, and its bytes as raw I/Q --
// and checks that the narrow width is the wide one narrowed, bit for bit. Each
// decodes the bytes straight to its own width, so this is what pins the two
// to the same answer.
func TestFixturesAtEveryWidth(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.wav"))
	if err != nil || len(files) == 0 {
		t.Skip("no fixtures alongside the test binary")
	}
	open := func(t *testing.T, path string, opt Options) *Reader {
		t.Helper()
		r, err := Open(path, opt)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			info := open(t, path, Options{}).Info()
			for c := MixChannels; int(c) < info.Channels; c++ {
				opt := Options{Channel: c}
				wide := drain[float64](t, open(t, path, opt), 7)
				narrowsTo(t, fmt.Sprint("channel ", c), wide, drain[float32](t, open(t, path, opt), 7), f64to32)
			}

			wide := drainPlanar[float64](t, open(t, path, Options{}), info.Channels, 7)
			got := drainPlanar[float32](t, open(t, path, Options{}), info.Channels, 7)
			for c := range wide {
				narrowsTo(t, fmt.Sprint("plane ", c), wide[c], got[c], f64to32)
			}

			// The whole file, header included, as I/Q: every byte is a sample, so
			// the float formats meet NaN and infinities.
			iq := Options{Container: Raw, Format: info.Format, Rate: 1, IQ: true}
			wideC := drain[complex128](t, open(t, path, iq), 7)
			narrowsTo(t, "raw I/Q", wideC, drain[complex64](t, open(t, path, iq), 7), c128to64)
		})
	}
}

// TestReadPlanarIQ reads two interleaved I/Q pairs into one complex plane per
// pair and checks each against a Reader selecting that pair.
func TestReadPlanarIQ(t *testing.T) {
	raw := make([]byte, 4*2*301)
	for i := range raw {
		raw[i] = byte(i*29 + 5)
	}
	opt := Options{Format: pcm.I16LE, Rate: 8000, Channels: 4, IQ: true}
	open := func(o Options) *Reader {
		r, err := NewReader(bytes.NewReader(raw), o)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	got := drainPlanar[complex128](t, open(opt), 2, 13)
	for pair := range Channel(2) {
		o := opt
		o.Channel = pair
		want := drain[complex128](t, open(o), 64)
		narrowsTo(t, fmt.Sprint("pair ", pair), want, got[pair], func(v complex128) complex128 { return v })
	}

	got64 := drainPlanar[complex64](t, open(opt), 2, 13)
	for c := range got {
		narrowsTo(t, "complex64 plane", got[c], got64[c], c128to64)
	}

	if _, err := open(opt).ReadPlanar(make([][]complex128, 4)); err == nil {
		t.Error("one plane per channel of an I/Q stream: want an error, since a plane is a pair")
	}
}

// TestFloatOverFullScaleNoteAtEveryWidth checks that the Note for a float
// sample past full scale is raised whatever width the samples are read at.
func TestFloatOverFullScaleNoteAtEveryWidth(t *testing.T) {
	raw := make([]byte, 0, 4*4)
	for _, v := range []float32{0.5, -0.25, 0.1, -2.0} {
		raw = append(raw, byte(math.Float32bits(v)), byte(math.Float32bits(v)>>8),
			byte(math.Float32bits(v)>>16), byte(math.Float32bits(v)>>24))
	}
	for _, tc := range []struct {
		name string
		iq   bool
		read func(*Reader) error
	}{
		{"float32", false, func(r *Reader) error { _, err := r.ReadSamples(make([]float32, 4)); return err }},
		{"float64", false, func(r *Reader) error { _, err := r.ReadSamples(make([]float64, 4)); return err }},
		{"complex64", true, func(r *Reader) error { _, err := r.ReadSamples(make([]complex64, 2)); return err }},
		{"complex128", true, func(r *Reader) error { _, err := r.ReadSamples(make([]complex128, 2)); return err }},
		{"float32 planar", false, func(r *Reader) error {
			_, err := r.ReadPlanar([][]float32{make([]float32, 2), make([]float32, 2)})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := Options{Format: pcm.F32LE, Rate: 8000, IQ: tc.iq}
			if tc.name == "float32 planar" {
				opt.Channels = 2
			}
			r, err := NewReader(bytes.NewReader(raw), opt)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.read(r); err != nil {
				t.Fatal(err)
			}
			if noteAbout(r.Info().Notes, "full scale") == "" {
				t.Errorf("no note about full scale in %q", r.Info().Notes)
			}
		})
	}
}

func TestContainer(t *testing.T) {
	for c, want := range map[Container]string{DetectContainer: "detect", WAV: "wav", Raw: "raw", 7: "Container(7)"} {
		if got := c.String(); got != want {
			t.Errorf("Container(%d).String() = %q, want %q", int(c), got, want)
		}
	}

	raw := i16(0.5, -0.5)
	if _, err := NewReader(bytes.NewReader(raw), Options{
		Format: pcm.I16LE, Rate: 8000, Container: WAV}); err == nil {
		t.Error("Container WAV accepted a stream with no header")
	}
	if _, err := NewReader(bytes.NewReader(raw), Options{
		Format: pcm.I16LE, Rate: 8000, Container: 7}); err == nil {
		t.Error("an unknown Container was accepted")
	}
}
