package riff

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// seekBuffer is an in-memory io.WriteSeeker, the seekable case the Writer
// patches sizes in.
type seekBuffer struct {
	buf []byte
	off int64
}

func (b *seekBuffer) Write(p []byte) (int, error) {
	if grow := b.off + int64(len(p)) - int64(len(b.buf)); grow > 0 {
		b.buf = append(b.buf, make([]byte, grow)...)
	}
	copy(b.buf[b.off:], p)
	b.off += int64(len(p))
	return len(p), nil
}

func (b *seekBuffer) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += b.off
	case io.SeekEnd:
		offset += int64(len(b.buf))
	}
	if offset < 0 {
		return 0, errors.New("negative offset")
	}
	b.off = offset
	return offset, nil
}

func newWriter(t *testing.T, w io.Writer) *Writer {
	t.Helper()
	wr, err := NewWriter(w, NewFourCC("WAVE"))
	if err != nil {
		t.Fatal(err)
	}
	return wr
}

// readBack parses a written container and returns each chunk's ID and data.
func readBack(t *testing.T, file []byte, unsized FourCC) ([]string, [][]byte) {
	t.Helper()
	r, err := New(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.AllowUnsizedChunk = unsized
	if r.FormType.String() != "WAVE" {
		t.Fatalf("FormType = %q", r.FormType)
	}
	var ids []string
	var data [][]byte
	for {
		c, err := r.Next()
		if errors.Is(err, io.EOF) {
			return ids, data
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		b, err := c.Bytes(1 << 20)
		if err != nil {
			t.Fatalf("chunk %s: %v", c.ID, err)
		}
		ids = append(ids, c.ID.String())
		data = append(data, b)
	}
}

func TestWriterRoundTrip(t *testing.T) {
	var b seekBuffer
	w := newWriter(t, &b)
	if err := w.WriteChunk(NewFourCC("fmt "), []byte("0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	c, err := w.Chunk(NewFourCC("data"))
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"abc", "def", "ghij"} {
		if n, err := io.WriteString(c, part); err != nil || n != len(part) {
			t.Fatalf("write %q: %d %v", part, n, err)
		}
	}
	if got := c.Size(); got != 10 {
		t.Errorf("Size = %d, want 10", got)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// The container size is patched and covers everything after it.
	if got := binary.LittleEndian.Uint32(b.buf[4:8]); got != uint32(len(b.buf)-8) {
		t.Errorf("container size = %d, want %d", got, len(b.buf)-8)
	}
	ids, data := readBack(t, b.buf, FourCC{})
	if want := []string{"fmt ", "data"}; len(ids) != 2 || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("chunks = %q, want %q", ids, want)
	}
	if string(data[0]) != "0123456789abcdef" || string(data[1]) != "abcdefghij" {
		t.Errorf("data = %q", data)
	}
}

// An odd-length chunk gets a pad byte that the declared size excludes, so the
// container stays on even boundaries and re-parses cleanly.
func TestWriterPadsOddChunks(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "WriteChunk"
		if streaming {
			name = "Chunk"
		}
		t.Run(name, func(t *testing.T) {
			var b seekBuffer
			w := newWriter(t, &b)
			if streaming {
				c, err := w.Chunk(NewFourCC("odd "))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := c.Write([]byte("abc")); err != nil {
					t.Fatal(err)
				}
			} else if err := w.WriteChunk(NewFourCC("odd "), []byte("abc")); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteChunk(NewFourCC("data"), []byte("wxyz")); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}

			if len(b.buf)%2 != 0 {
				t.Errorf("file is %d bytes, want an even length", len(b.buf))
			}
			if got := binary.LittleEndian.Uint32(b.buf[16:20]); got != 3 {
				t.Errorf("odd chunk size = %d, want 3 (the pad byte is not counted)", got)
			}
			ids, data := readBack(t, b.buf, FourCC{})
			if len(ids) != 2 || ids[0] != "odd " || ids[1] != "data" {
				t.Fatalf("chunks = %q", ids)
			}
			if string(data[0]) != "abc" || string(data[1]) != "wxyz" {
				t.Errorf("data = %q", data)
			}
		})
	}
}

// Without a seek, the sizes stay unpatched — what a streaming encoder emits and
// what AllowUnsizedChunk reads back.
func TestWriterUnseekable(t *testing.T) {
	var b bytes.Buffer
	w := newWriter(t, &b)
	if err := w.WriteChunk(NewFourCC("fmt "), []byte("0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	c, err := w.Chunk(NewFourCC("data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	file := b.Bytes()
	if got := binary.LittleEndian.Uint32(file[4:8]); got != 0xFFFFFFFF {
		t.Errorf("container size = %#x, want 0xFFFFFFFF", got)
	}
	if got := binary.LittleEndian.Uint32(file[40:44]); got != 0xFFFFFFFF {
		t.Errorf("data size = %#x, want 0xFFFFFFFF", got)
	}
	// The sized fmt chunk is still correct, since its length was known.
	if got := binary.LittleEndian.Uint32(file[16:20]); got != 16 {
		t.Errorf("fmt size = %d, want 16", got)
	}

	ids, data := readBack(t, file, NewFourCC("data"))
	if len(ids) != 2 || ids[1] != "data" || string(data[1]) != "abcdef" {
		t.Errorf("chunks = %q data = %q", ids, data)
	}
}

func TestWriterStaleChunkWriter(t *testing.T) {
	var b seekBuffer
	w := newWriter(t, &b)
	first, err := w.Chunk(NewFourCC("one "))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}

	// Opening the next chunk closes this one.
	if _, err := w.Chunk(NewFourCC("two ")); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte("cd")); !errors.Is(err, ErrStaleChunk) {
		t.Errorf("write to the previous chunk = %v, want ErrStaleChunk", err)
	}
	// Closing it again is a no-op rather than a second pad byte.
	if err := first.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	ids, data := readBack(t, b.buf, FourCC{})
	if len(ids) != 2 || ids[0] != "one " || string(data[0]) != "ab" {
		t.Errorf("chunks = %q data = %q", ids, data)
	}
}

// Close is idempotent, and neither Close nor the chunk writers touch Close on
// the underlying writer.
func TestWriterCloseDoesNotCloseUnderlying(t *testing.T) {
	var rec closeRecorder
	w := newWriter(t, &rec)
	if err := w.WriteChunk(NewFourCC("data"), []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
	if rec.closes != 0 {
		t.Errorf("underlying Close called %d times, want 0", rec.closes)
	}
	if _, err := w.Chunk(NewFourCC("more")); !errors.Is(err, ErrClosed) {
		t.Errorf("Chunk after Close = %v, want ErrClosed", err)
	}
	if err := w.WriteChunk(NewFourCC("more"), nil); !errors.Is(err, ErrClosed) {
		t.Errorf("WriteChunk after Close = %v, want ErrClosed", err)
	}
}

type closeRecorder struct {
	seekBuffer
	closes int
}

func (c *closeRecorder) Close() error {
	c.closes++
	return nil
}

// A size past what the header field can express is refused rather than wrapped.
func TestWriterRejectsOversizedChunk(t *testing.T) {
	var b seekBuffer
	w := newWriter(t, &b)
	// Start near the limit rather than writing four gigabytes.
	w.n = maxSize - 8
	if err := w.WriteChunk(NewFourCC("data"), []byte("abcdefgh")); !errors.Is(err, ErrChunkTooLarge) {
		t.Errorf("WriteChunk near the limit = %v, want ErrChunkTooLarge", err)
	}
	// The error sticks: the container is no longer writable.
	if err := w.Close(); !errors.Is(err, ErrChunkTooLarge) {
		t.Errorf("Close = %v, want ErrChunkTooLarge", err)
	}
}

// A writer that fails outright is reported by NewWriter, not stashed.
func TestWriterHeaderError(t *testing.T) {
	if _, err := NewWriter(&failAfter{}, NewFourCC("WAVE")); err == nil {
		t.Fatal("NewWriter succeeded on a failing writer")
	}
}

// A failure after the header is recorded once and reported by every later call.
func TestWriterErrorSticks(t *testing.T) {
	w, err := NewWriter(&failAfter{remaining: 12}, NewFourCC("WAVE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteChunk(NewFourCC("data"), []byte("abc")); err == nil {
		t.Fatal("WriteChunk succeeded on a failing writer")
	}
	if err := w.Close(); err == nil {
		t.Error("Close reported no error")
	}
}

// failAfter accepts remaining bytes and fails on whatever comes next.
type failAfter struct{ remaining int }

func (f *failAfter) Write(p []byte) (int, error) {
	if len(p) > f.remaining {
		n := f.remaining
		f.remaining = 0
		return n, errors.New("write failed")
	}
	f.remaining -= len(p)
	return len(p), nil
}

// An unsized chunk reads to EOF, so a pad byte after it would come back as
// data. It must not be written, and the chunk must be the container's last.
func TestWriterUnsizedOddChunkIsNotPadded(t *testing.T) {
	var b bytes.Buffer
	w, err := NewWriter(&b, NewFourCC("WAVE"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := w.Chunk(NewFourCC("data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if got, want := len(b.Bytes()), 12+8+3; got != want {
		t.Errorf("file is %d bytes, want %d with no pad", got, want)
	}
	_, data := readBack(t, b.Bytes(), NewFourCC("data"))
	if len(data) != 1 || string(data[0]) != "abc" {
		t.Errorf("data = %q, want [\"abc\"]", data)
	}
}

// Nothing may follow a chunk whose size could not be patched, since a reader
// consumes the rest of the stream as that chunk's data.
func TestWriterRejectsChunkAfterUnsized(t *testing.T) {
	var b bytes.Buffer
	w, err := NewWriter(&b, NewFourCC("WAVE"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := w.Chunk(NewFourCC("data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Chunk(NewFourCC("more")); !errors.Is(err, ErrUnsizedNotLast) {
		t.Errorf("Chunk after an unsized chunk = %v, want ErrUnsizedNotLast", err)
	}
	if err := w.WriteChunk(NewFourCC("more"), []byte("x")); !errors.Is(err, ErrUnsizedNotLast) {
		t.Errorf("WriteChunk after an unsized chunk = %v, want ErrUnsizedNotLast", err)
	}
	// The container itself is still valid and closes cleanly.
	if err := w.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
	_, data := readBack(t, b.Bytes(), NewFourCC("data"))
	if len(data) != 1 || string(data[0]) != "abcd" {
		t.Errorf("data = %q", data)
	}
}

// A seekable writer patches the size, so the pad byte is still required.
func TestWriterSeekableOddChunkIsPadded(t *testing.T) {
	var b seekBuffer
	w := newWriter(t, &b)
	c, err := w.Chunk(NewFourCC("data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := len(b.buf), 12+8+4; got != want {
		t.Errorf("file is %d bytes, want %d including the pad", got, want)
	}
	_, data := readBack(t, b.buf, FourCC{})
	if len(data) != 1 || string(data[0]) != "abc" {
		t.Errorf("data = %q", data)
	}
}
