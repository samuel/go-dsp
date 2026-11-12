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

	r, err := NewReader(bytes.NewReader(raw), Layout{})
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

// TestSeekFrameIsAbsolute checks that frames are counted from the start of
// the stream wherever the Reader is, and that a seek is bounded by its length.
func TestSeekFrameIsAbsolute(t *testing.T) {
	payload, want := ramp(8)
	path := filepath.Join(t.TempDir(), "ramp.wav")
	if err := os.WriteFile(path, wav(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path, Layout{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on

	if n, err := r.Skip(2); n != 2 || err != nil {
		t.Fatalf("Skip(2) = %d, %v", n, err)
	}
	seekTo(t, r, 1)
	if got := readAll(t, r, 3); !slices.Equal(got, want[1:]) {
		t.Errorf("after SeekFrame(1) read %v, want %v", got, want[1:])
	}
	seekTo(t, r, r.Info().Frames)
	if got := readAll(t, r, 3); len(got) != 0 {
		t.Errorf("read %v at the end", got)
	}
	if err := r.SeekFrame(9); err == nil {
		t.Error("SeekFrame(9) reached past the 8 frames of the file")
	}
}

// skipSources opens the same samples as each kind of source Skip treats
// differently. The WAV ones have a chunk after the samples, which a skip must
// not run into.
func skipSources(t *testing.T, payload []byte) map[string]func() *Reader {
	t.Helper()
	raw := Layout{Format: pcm.I16LE, Rate: 8000}
	file := wavTrailing(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload, nil,
		[][]byte{chunk("LIST", make([]byte, 64))})
	path := filepath.Join(t.TempDir(), "skip.wav")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	must := func(r *Reader, err error) *Reader {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	return map[string]func() *Reader{
		"a file":            func() *Reader { return must(Open(path, Layout{})) },
		"a seekable stream": func() *Reader { return must(NewReader(bytes.NewReader(payload), raw)) },
		"a seekable WAV":    func() *Reader { return must(NewReader(bytes.NewReader(file), Layout{})) },
		"a pipe":            func() *Reader { return must(NewReader(struct{ io.Reader }{bytes.NewReader(payload)}, raw)) },
		"a piped WAV":       func() *Reader { return must(NewReader(struct{ io.Reader }{bytes.NewReader(file)}, Layout{})) },
	}
}

// TestSkip checks that Skip lands on the right frame and reports the end,
// whether it seeks or reads, and whether the frames are already buffered or
// lie more than a block ahead.
func TestSkip(t *testing.T) {
	// Longer than the Reader's block, so one skip crosses several.
	const frames = 100_000
	want := make([]float64, frames)
	for i := range want {
		want[i] = float64(i%2000-1000) / 1024
	}
	for name, open := range skipSources(t, i16(want...)) {
		t.Run(name, func(t *testing.T) {
			r := open()
			one := make([]float64, 1)
			at := 0
			next := func() {
				t.Helper()
				if n, err := r.ReadSamples(one); n != 1 || err != nil {
					t.Fatalf("ReadSamples at frame %d = %d, %v", at, n, err)
				}
				if one[0] != want[at] {
					t.Fatalf("frame %d = %v, want %v", at, one[0], want[at])
				}
				at++
			}
			skip := func(n int64) {
				t.Helper()
				if got, err := r.Skip(n); got != n || err != nil {
					t.Fatalf("Skip(%d) at frame %d = %d, %v", n, at, got, err)
				}
				at += int(n)
			}

			skip(0)
			next()
			skip(2) // inside what the first read buffered
			next()
			skip(70_000) // past the buffer
			next()
			skip(3)
			next()

			if _, err := r.Skip(-1); err == nil {
				t.Error("Skip(-1) succeeded")
			}
			// Past the end: the count is what was there, and the trailing
			// chunk of the WAV sources is not counted as samples.
			left := int64(frames - at)
			if got, err := r.Skip(left + 50); got != left || !errors.Is(err, io.EOF) {
				t.Errorf("Skip past the end = %d, %v, want %d, io.EOF", got, err, left)
			}
			if n, err := r.ReadSamples(one); n != 0 || !errors.Is(err, io.EOF) {
				t.Errorf("a read after the last frame = %d, %v, want io.EOF", n, err)
			}
			if got, err := r.Skip(1); got != 0 || !errors.Is(err, io.EOF) {
				t.Errorf("Skip at the end = %d, %v, want 0, io.EOF", got, err)
			}
		})
	}
}

// TestSkipToTheEnd checks that skipping exactly the frames left is not an
// error: the end is reported only when a frame asked for is not there.
func TestSkipToTheEnd(t *testing.T) {
	payload, _ := ramp(8)
	for name, open := range skipSources(t, payload) {
		t.Run(name, func(t *testing.T) {
			r := open()
			if got, err := r.Skip(8); got != 8 || err != nil {
				t.Errorf("Skip(8) of 8 frames = %d, %v", got, err)
			}
		})
	}
}

// TestSkipCountsFrames checks that a skip of a stereo stream moves whole
// frames, not samples.
func TestSkipCountsFrames(t *testing.T) {
	raw := i16(0, 0.5, 0.125, 0.5, 0.25, 0.5, 0.375, 0.5) // four stereo frames
	for _, src := range []io.Reader{bytes.NewReader(raw), struct{ io.Reader }{bytes.NewReader(raw)}} {
		r, err := NewReader(src, Layout{Format: pcm.I16LE, Rate: 8000, Channels: 2})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := r.Skip(1); n != 1 || err != nil {
			t.Fatalf("Skip(1) = %d, %v", n, err)
		}
		got := readAllPlanar(t, r, 2, 3)
		if !slices.Equal(got[0], []float64{0.125, 0.25, 0.375}) {
			t.Errorf("left channel after the skip = %v", got[0])
		}
	}
}

// TestSeekFramePlanar seeks a stereo file and a headerless I/Q stream, where a
// frame is more than one element.
func TestSeekFramePlanar(t *testing.T) {
	payload, want := ramp(12) // six stereo frames, or six I/Q pairs

	r, err := NewReader(bytes.NewReader(wav(pcmFmt(wavPCM, 2, 8000, 16), uint32(len(payload)), payload)), Layout{})
	if err != nil {
		t.Fatal(err)
	}
	readAllPlanar(t, r, 2, 4)
	seekTo(t, r, 4)
	got := readAllPlanar(t, r, 2, 4)
	if !slices.Equal(got[0], []float64{want[8], want[10]}) || !slices.Equal(got[1], []float64{want[9], want[11]}) {
		t.Errorf("after SeekFrame(4) read %v", got)
	}

	iq, err := NewReader(bytes.NewReader(payload), Layout{Format: pcm.I16LE, Rate: 8000, IQ: true})
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
	r, err := NewReader(bytes.NewReader(wav(pcmFmt(wavPCM, 1, 8000, 16), 0xffffffff, payload)), Layout{})
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
	r, err := NewReader(struct{ io.Reader }{bytes.NewReader(raw)}, Layout{})
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

	r, err := NewReader(bytes.NewReader(raw), Layout{})
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

	headerless, err := NewReader(bytes.NewReader(payload), Layout{Format: pcm.I16LE, Rate: 8000})
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
		wavTrailing(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload, before, nil)), Layout{})
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
			opt := Layout{
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
			r, err := NewReader(bytes.NewReader(b), Layout{})
			if err != nil {
				t.Fatal(err)
			}
			if got := r.Info().ChannelMask; got != tc.want {
				t.Errorf("Reader mask %#x, want %#x", got, tc.want)
			}
		})
	}
}
