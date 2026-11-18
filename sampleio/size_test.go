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

// statOnly is a source that reports its size but cannot seek, as a file in a
// zip archive does.
type statOnly struct {
	io.Reader
	f *os.File
}

func (s statOnly) Stat() (os.FileInfo, error) { return s.f.Stat() }

// statFails is a seekable source whose Stat returns an error.
type statFails struct{ io.ReadSeeker }

func (statFails) Stat() (os.FileInfo, error) { return nil, errors.New("no stat") }

// TestNewReaderSize checks that a source the caller opened gives the same
// length as Open: from a file, an fs.FS, a source that only reports its size,
// and from the middle of a file.
func TestNewReaderSize(t *testing.T) {
	payload, _ := ramp(8)
	dir := t.TempDir()
	path := filepath.Join(dir, "x.raw")
	if err := os.WriteFile(path, slices.Concat(make([]byte, 6), payload), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := Layout{Format: pcm.I16LE, Rate: 8000}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read only
	if r, err := NewReader(f, raw); err != nil || r.Info().Frames != 11 {
		t.Errorf("an *os.File: %v, %v; want 11 frames", frames(r), err)
	}

	// From an offset, the size counts from there.
	if _, err := f.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader(f, raw)
	if err != nil || r.Info().Frames != 8 {
		t.Fatalf("an *os.File at 6 bytes: %v, %v; want 8 frames", frames(r), err)
	}
	// The end is known, so a seek past it is refused.
	if err := r.SeekFrame(9); err == nil {
		t.Error("SeekFrame past the end of a file from NewReader succeeded")
	}

	ff, err := os.DirFS(dir).Open("x.raw")
	if err != nil {
		t.Fatal(err)
	}
	defer ff.Close() //nolint:errcheck // read only
	if r, err := NewReader(ff, raw); err != nil || r.Info().Frames != 11 {
		t.Errorf("an fs.File: %v, %v; want 11 frames", frames(r), err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if r, err := NewReader(statOnly{f, f}, raw); err != nil || r.Info().Frames != 11 {
		t.Errorf("a source with only Stat: %v, %v; want 11 frames", frames(r), err)
	}

	// A Stat that fails falls back to seeking.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if r, err := NewReader(statFails{f}, raw); err != nil || r.Info().Frames != 11 {
		t.Errorf("a source whose Stat fails: %v, %v; want 11 frames", frames(r), err)
	}
}

// TestNewReaderOversizedHeader checks that a WAV whose header claims more data
// than the file holds is read to the end, as with Open.
func TestNewReaderOversizedHeader(t *testing.T) {
	payload, want := ramp(8)
	path := filepath.Join(t.TempDir(), "x.wav")
	if err := os.WriteFile(path, wav(pcmFmt(wavPCM, 1, 8000, 16), 1<<20, payload), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read only
	r, err := NewReader(f, Layout{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Info(); got.Frames != -1 || len(got.Notes) == 0 {
		t.Errorf("Info = %+v, want an unknown length and a Note", got)
	}
	if got := readAll(t, r, 3); len(got) != len(want) {
		t.Errorf("read %d frames, want %d", len(got), len(want))
	}
}

// TestWAVDataSizeAfterHeader checks a data size against the bytes after the
// header rather than the whole file: a claim of 10 bytes in a file that holds
// 46, of which 2 are samples, is too large.
func TestWAVDataSizeAfterHeader(t *testing.T) {
	payload := i16(0.5)
	in := wav(pcmFmt(wavPCM, 1, 8000, 16), 10, payload)
	if len(in) != 46 {
		t.Fatalf("the file is %d bytes, want 46", len(in))
	}
	for _, tc := range []struct {
		name   string
		claim  uint32
		frames int64
	}{
		{"exact", 2, 1},
		{"slightly over", 10, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := wav(pcmFmt(wavPCM, 1, 8000, 16), tc.claim, payload)
			r, err := NewReader(bytes.NewReader(in), Layout{})
			if err != nil {
				t.Fatal(err)
			}
			if got := r.Info().Frames; got != tc.frames {
				t.Errorf("Frames = %d, want %d", got, tc.frames)
			}
			if got := len(readAll(t, r, 3)); got != 1 {
				t.Errorf("read %d frames, want 1", got)
			}
		})
	}
}

func TestCheckWritable(t *testing.T) {
	for _, l := range []Layout{
		{Format: pcm.I16LE, Rate: 8000},
		{Format: pcm.U8, Rate: 8000, IQ: true, Container: Raw},
	} {
		if err := l.CheckWritable(); err != nil {
			t.Errorf("%+v: %v", l, err)
		}
	}
	for _, l := range []Layout{
		{Rate: 8000},
		{Format: pcm.I16LE, Rate: 8000, IQ: true, Channels: 4},
	} {
		_, werr := NewWriter(io.Discard, l)
		if err := l.CheckWritable(); err == nil || werr == nil || err.Error() != werr.Error() {
			t.Errorf("%+v: CheckWritable = %v, NewWriter = %v; want the same error", l, err, werr)
		}
	}
}

func frames(r *Reader) any {
	if r == nil {
		return nil
	}
	return r.Info().Frames
}
