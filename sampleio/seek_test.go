package sampleio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"samuelks.com/dsp/pcm"
)

// ramp is n frames whose value names their index, so a read after a seek says
// which frame it started at.
func ramp(n int) ([]byte, []float64) {
	want := make([]float64, n)
	for i := range want {
		want[i] = float64(i) / 64
	}
	return i16(want...), want
}

func seekTo(t *testing.T, r *Reader, n int64) {
	t.Helper()
	if err := r.SeekFrame(n); err != nil {
		t.Fatalf("SeekFrame(%d): %v", n, err)
	}
}

// TestSeekFrame seeks within a WAV whose data is followed by another chunk,
// which a seek must leave unread just as a straight read does.
func TestSeekFrame(t *testing.T) {
	payload, want := ramp(8)
	trailer := chunk("LIST", []byte("INFOISFTsomething"))
	raw := wavTrailing(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload, nil, [][]byte{trailer})

	r, err := NewReader(bytes.NewReader(raw), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Part of the way in, so the seek has buffered frames to drop.
	if _, err := r.ReadSamples(make([]float64, 3)); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int64{5, 0, 7, 8, 2} {
		seekTo(t, r, n)
		if got := readAll(t, r, 3); !slices.Equal(got, want[n:]) {
			t.Errorf("after SeekFrame(%d) read %v, want %v", n, got, want[n:])
		}
	}
	for _, n := range []int64{-1, 9} {
		if err := r.SeekFrame(n); err == nil {
			t.Errorf("SeekFrame(%d) of 8 frames succeeded", n)
		}
	}
	// A refused seek leaves the Reader where it was: at the end.
	if got := readAll(t, r, 3); len(got) != 0 {
		t.Errorf("read %v after a refused seek, want nothing", got)
	}
}

// TestSeekFrameWindow checks that frames are counted from Skip and bounded by
// Limit, so a seek cannot reach outside what the Reader was opened to produce.
func TestSeekFrameWindow(t *testing.T) {
	payload, want := ramp(8)
	path := filepath.Join(t.TempDir(), "ramp.wav")
	if err := os.WriteFile(path, wav(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path, Options{Skip: 2, Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on

	if got := readAll(t, r, 3); !slices.Equal(got, want[2:6]) {
		t.Fatalf("read %v, want %v", got, want[2:6])
	}
	seekTo(t, r, 1)
	if got := readAll(t, r, 3); !slices.Equal(got, want[3:6]) {
		t.Errorf("after SeekFrame(1) read %v, want %v", got, want[3:6])
	}
	seekTo(t, r, r.Info().Frames)
	if got := readAll(t, r, 3); len(got) != 0 {
		t.Errorf("read %v at the end", got)
	}
	if err := r.SeekFrame(5); err == nil {
		t.Error("SeekFrame(5) reached past a Limit of 4")
	}
}

// TestSeekFramePlanar seeks a stereo file and a headerless I/Q stream, where a
// frame is more than one element.
func TestSeekFramePlanar(t *testing.T) {
	payload, want := ramp(12) // six stereo frames, or six I/Q pairs

	r, err := NewReader(bytes.NewReader(wav(pcmFmt(wavPCM, 2, 8000, 16), uint32(len(payload)), payload)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	readAllPlanar(t, r, 2, 4)
	seekTo(t, r, 4)
	got := readAllPlanar(t, r, 2, 4)
	if !slices.Equal(got[0], []float64{want[8], want[10]}) || !slices.Equal(got[1], []float64{want[9], want[11]}) {
		t.Errorf("after SeekFrame(4) read %v", got)
	}

	iq, err := NewReader(bytes.NewReader(payload), Options{Format: pcm.I16LE, Rate: 8000, IQ: true})
	if err != nil {
		t.Fatal(err)
	}
	seekTo(t, iq, 5)
	pairs := drain[complex128](t, iq, 4)
	if len(pairs) != 1 || pairs[0] != complex(want[10], want[11]) {
		t.Errorf("after SeekFrame(5) read %v", pairs)
	}
}

// TestSeekFrameUnknownLength is a stream written down a pipe and saved: its
// header states no size, so there is no end to check a frame against.
func TestSeekFrameUnknownLength(t *testing.T) {
	payload, want := ramp(8)
	r, err := NewReader(bytes.NewReader(wav(pcmFmt(wavPCM, 1, 8000, 16), 0xffffffff, payload)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	seekTo(t, r, 6)
	if got := readAll(t, r, 3); !slices.Equal(got, want[6:]) {
		t.Errorf("after SeekFrame(6) read %v, want %v", got, want[6:])
	}
	seekTo(t, r, 100)
	if got := readAll(t, r, 3); len(got) != 0 {
		t.Errorf("read %v past the end", got)
	}
	if err := r.SeekFrame(1 << 62); err == nil {
		t.Error("SeekFrame took a frame whose offset overflows")
	}
}

func TestSeekFrameNotSeekable(t *testing.T) {
	payload, _ := ramp(8)
	raw := wav(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload)
	// Wrapping the reader hides its Seek, as a pipe has none worth calling.
	r, err := NewReader(struct{ io.Reader }{bytes.NewReader(raw)}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SeekFrame(0); !errors.Is(err, ErrNotSeekable) {
		t.Errorf("SeekFrame on a stream: %v, want ErrNotSeekable", err)
	}
}

// TestWAVChunks checks that the chunks ahead of the samples are kept, that one
// too large to keep is listed without its data, and that neither a chunk after
// the samples nor the format chunk is listed.
func TestWAVChunks(t *testing.T) {
	payload, want := ramp(4)
	before := [][]byte{
		chunk("LIST", []byte{1, 2, 3}),
		chunk("big ", make([]byte, maxChunkSize+1)),
		chunk("cue ", nil),
	}
	after := [][]byte{chunk("id3 ", []byte("late"))}
	raw := wavTrailing(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload, before, after)

	r, err := NewReader(bytes.NewReader(raw), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Chunks()
	if len(got) != 3 {
		t.Fatalf("%d chunks, want 3: %v", len(got), got)
	}
	if got[0].ID.String() != "LIST" || got[0].Size != 3 || !bytes.Equal(got[0].Data, []byte{1, 2, 3}) {
		t.Errorf("chunk 0 = %v %d %v", got[0].ID, got[0].Size, got[0].Data)
	}
	if got[1].ID.String() != "big " || got[1].Size != maxChunkSize+1 || got[1].Data != nil {
		t.Errorf("chunk 1 = %v %d with %d bytes kept, want it listed without data", got[1].ID, got[1].Size, len(got[1].Data))
	}
	if got[2].ID.String() != "cue " || got[2].Data == nil || len(got[2].Data) != 0 {
		t.Errorf("chunk 2 = %v %v, want an empty chunk kept as empty", got[2].ID, got[2].Data)
	}
	if samples := readAll(t, r, 3); !slices.Equal(samples, want) {
		t.Errorf("read %v, want %v", samples, want)
	}

	headerless, err := NewReader(bytes.NewReader(payload), Options{Format: pcm.I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if c := headerless.Chunks(); c != nil {
		t.Errorf("a headerless stream has chunks: %v", c)
	}
}

// TestWAVChunkTotal checks the cap on what all the kept chunks may add up to.
func TestWAVChunkTotal(t *testing.T) {
	payload, _ := ramp(4)
	var before [][]byte
	for range maxChunkTotal/maxChunkSize + 1 {
		before = append(before, chunk("LIST", make([]byte, maxChunkSize)))
	}
	r, err := NewReader(bytes.NewReader(
		wavTrailing(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload, before, nil)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Chunks()
	for i, c := range got {
		if last := i == len(got)-1; (c.Data == nil) != last {
			t.Errorf("chunk %d of %d: data kept = %v", i, len(got), c.Data != nil)
		}
	}
}

// TestWAVChannelMask checks the speaker mask in both directions: what a header
// states is reported, and what a Writer is given is written.
func TestWAVChannelMask(t *testing.T) {
	for _, tc := range []struct {
		name     string
		f        pcm.Format
		channels int
		mask     uint32
		want     uint32
		tag      uint16
	}{
		{"none", pcm.I16LE, 2, 0, 0, wavPCM},
		{"given", pcm.I16LE, 2, 0x30, 0x30, wavExtensible},
		{"default for an extensible header", pcm.I24LE, 1, 0, maskMono, wavExtensible},
		{"given over the default", pcm.I24LE, 2, 0x600, 0x600, wavExtensible},
		{"surround", pcm.F32LE, 6, 0x3f, 0x3f, wavExtensible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := WriterOptions{
				Format: tc.f, Rate: 8000, Channels: tc.channels,
				ChannelMask: tc.mask,
			}
			planes := make([][]float32, tc.channels)
			for c := range planes {
				planes[c] = make([]float32, 5)
			}
			b := writeAll(t, opt, false, func(w *Writer) (int, error) {
				if got := w.Info().ChannelMask; got != tc.mask {
					t.Errorf("Writer mask %#x, want %#x", got, tc.mask)
				}
				return w.WritePlanar(planes)
			})
			fmtAt := bytes.Index(b, []byte("fmt "))
			if tag := uint16(b[fmtAt+8]) | uint16(b[fmtAt+9])<<8; tag != tc.tag {
				t.Errorf("tag %#04x, want %#04x", tag, tc.tag)
			}
			r, err := NewReader(bytes.NewReader(b), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got := r.Info().ChannelMask; got != tc.want {
				t.Errorf("Reader mask %#x, want %#x", got, tc.want)
			}
		})
	}
}
