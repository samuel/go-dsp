package riff

import (
	"encoding/binary"
	"fmt"
	"io"
)

// maxSize is the largest size a header field can express. A size of exactly
// unsizedSize would be indistinguishable from one an encoder never patched.
const maxSize = unsizedSize - 1

// Writer assembles a RIFF container. Sizes are written as unsizedSize up front
// and patched on Close when the underlying writer can seek; otherwise they stay
// that way, which is what a streaming encoder emits and what
// Reader.AllowUnsizedChunk reads back. A chunk left unsized is the container's
// last, and carries no pad byte, since it reads to the end of the stream. Close
// does not close the underlying writer.
type Writer struct {
	w    io.Writer
	ws   io.WriteSeeker // non-nil when w can seek, and patching is possible
	base int64          // offset of the RIFF header
	n    int64          // bytes written after the container size field
	cur  *ChunkWriter
	err  error
	done bool

	// unsized records that a chunk was written with a size that could not be
	// patched, which ends the container: it reads to EOF.
	unsized bool
}

// NewWriter writes the container header and returns a Writer for its chunks.
// Close should be called once all chunks have been written to update the
// header to include the size.
func NewWriter(w io.Writer, formType FourCC) (*Writer, error) {
	wr := &Writer{w: w}
	if ws, ok := w.(io.WriteSeeker); ok {
		off, err := ws.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		wr.ws = ws
		wr.base = off
	}
	var hdr [12]byte
	copy(hdr[:], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:], unsizedSize)
	copy(hdr[8:], formType[:])
	if _, err := w.Write(hdr[:]); err != nil {
		return nil, err
	}
	wr.n = 4 // the form type the container size covers
	return wr, nil
}

// Chunk starts a chunk whose size is not known in advance and returns a writer
// for its data. Any chunk still open is closed first, so the returned writer is
// the only one that may be written to. Use WriteChunk when the data is already
// in hand: it writes a real size even to a writer that cannot seek.
//
// On a writer that cannot seek the size stays unpatched, so the chunk reads to
// the end of the stream and must be the last one: a later chunk is
// ErrUnsizedNotLast rather than output no reader could reach.
func (w *Writer) Chunk(id FourCC) (*ChunkWriter, error) {
	if err := w.closeChunk(); err != nil {
		return nil, err
	}
	if w.done {
		return nil, ErrClosed
	}
	if w.unsized {
		return nil, ErrUnsizedNotLast
	}
	if w.err != nil {
		return nil, w.err
	}
	sizeAt := w.base + 8 + w.n + 4
	var hdr [8]byte
	copy(hdr[:], id[:])
	binary.LittleEndian.PutUint32(hdr[4:], unsizedSize)
	if err := w.write(hdr[:]); err != nil {
		return nil, err
	}
	c := &ChunkWriter{ID: id, w: w, sizeAt: sizeAt}
	w.cur = c
	return c, nil
}

// WriteChunk writes a whole chunk, padding it to an even length. The size is
// known, so nothing needs patching and the result is correct on a writer that
// cannot seek.
func (w *Writer) WriteChunk(id FourCC, data []byte) error {
	if err := w.closeChunk(); err != nil {
		return err
	}
	if w.done {
		return ErrClosed
	}
	if w.unsized {
		return ErrUnsizedNotLast
	}
	if w.err != nil {
		return w.err
	}
	if int64(len(data)) > maxSize {
		return w.fail(fmt.Errorf("riff: chunk %s is %d bytes: %w", id, len(data), ErrChunkTooLarge))
	}
	var hdr [8]byte
	copy(hdr[:], id[:])
	binary.LittleEndian.PutUint32(hdr[4:], uint32(len(data)))
	if err := w.write(hdr[:]); err != nil {
		return err
	}
	if err := w.write(data); err != nil {
		return err
	}
	return w.pad(int64(len(data)))
}

// Close finishes the container, closing any open chunk and patching the sizes
// when the underlying writer can seek. It is idempotent and never closes the
// underlying writer. Without it, a seekable file keeps the unpatched sizes it
// was written with.
func (w *Writer) Close() error {
	if w.done {
		return w.err
	}
	if err := w.closeChunk(); err != nil {
		w.done = true
		return err
	}
	w.done = true
	if w.err != nil {
		return w.err
	}
	return w.patch(w.base+4, uint32(w.n))
}

// closeChunk closes the open chunk, if any.
func (w *Writer) closeChunk() error {
	if w.cur == nil {
		return nil
	}
	return w.cur.Close()
}

// write appends to the container, keeping the declared size within range.
func (w *Writer) write(p []byte) error {
	if w.err != nil {
		return w.err
	}
	if w.n+int64(len(p)) > maxSize {
		return w.fail(fmt.Errorf("riff: container exceeds %d bytes: %w", int64(maxSize), ErrChunkTooLarge))
	}
	if _, err := w.w.Write(p); err != nil {
		return w.fail(err)
	}
	w.n += int64(len(p))
	return nil
}

// pad writes the RIFF pad byte after an odd-length chunk. It counts toward the
// container size but not toward the chunk's own.
func (w *Writer) pad(n int64) error {
	if n%2 == 0 {
		return nil
	}
	return w.write([]byte{0})
}

// patch rewrites a size field and returns to the end of the container. It is a
// no-op when the underlying writer cannot seek, leaving the unsized value.
func (w *Writer) patch(at int64, size uint32) error {
	if w.ws == nil || w.err != nil {
		return w.err
	}
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], size)
	if _, err := w.ws.Seek(at, io.SeekStart); err != nil {
		return w.fail(err)
	}
	if _, err := w.ws.Write(buf[:]); err != nil {
		return w.fail(err)
	}
	if _, err := w.ws.Seek(w.base+8+w.n, io.SeekStart); err != nil {
		return w.fail(err)
	}
	return nil
}

// fail records the first error; every later call reports it.
func (w *Writer) fail(err error) error {
	if w.err == nil {
		w.err = err
	}
	return w.err
}

// ChunkWriter writes the data of one streaming chunk. It is valid until it is
// closed or the parent Writer moves on to the next chunk.
type ChunkWriter struct {
	// ID identifies the chunk being written.
	ID FourCC

	w      *Writer
	sizeAt int64 // offset of the size field to patch
	n      int64 // data bytes written, excluding the pad
	closed bool
}

// Write implements io.Writer over the chunk's data. It returns ErrStaleChunk
// once the chunk is closed or the parent has moved on.
func (c *ChunkWriter) Write(p []byte) (int, error) {
	if err := c.check(); err != nil {
		return 0, err
	}
	if c.n+int64(len(p)) > maxSize {
		return 0, c.w.fail(fmt.Errorf("riff: chunk %s exceeds %d bytes: %w", c.ID, int64(maxSize), ErrChunkTooLarge))
	}
	if err := c.w.write(p); err != nil {
		return 0, err
	}
	c.n += int64(len(p))
	return len(p), nil
}

// Size is the number of data bytes written so far, excluding the pad byte.
func (c *ChunkWriter) Size() int64 { return c.n }

// Close pads the chunk to an even length and patches its size when the
// underlying writer can seek. It is idempotent.
func (c *ChunkWriter) Close() error {
	if c.closed {
		return c.w.err
	}
	c.closed = true
	if c.w.cur == c {
		c.w.cur = nil
	}
	if c.w.err != nil {
		return c.w.err
	}
	if c.w.ws == nil {
		// The size stays unpatched, so the chunk reads to EOF. The pad byte
		// aligns the following chunk header, and there is none — a reader with
		// no boundary to strip it would hand it back as data.
		c.w.unsized = true
		return nil
	}
	if err := c.w.pad(c.n); err != nil {
		return err
	}
	return c.w.patch(c.sizeAt, uint32(c.n))
}

func (c *ChunkWriter) check() error {
	if c.closed || c.w.cur != c {
		return ErrStaleChunk
	}
	return c.w.err
}
