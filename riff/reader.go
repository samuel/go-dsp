package riff

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// unsizedSize is the size written by encoders that cannot seek back to patch a real size.
const unsizedSize = 0xFFFFFFFF

// FourCC is a 4-character code used to identify RIFF chunks.
type FourCC [4]byte

// NewFourCC returns the code named by s, which is truncated or NUL-padded to
// four bytes.
func NewFourCC(s string) FourCC {
	var f FourCC
	copy(f[:], s)
	return f
}

func (f FourCC) String() string { return string(f[:]) }

// Reader iterates over the chunks of a RIFF container.
type Reader struct {
	// AllowUnsizedChunk names the one chunk whose declared size of zero or
	// 0xFFFFFFFF means "read to the end of the stream" rather than failing;
	// streaming encoders emit such sizes because they cannot seek back to patch
	// them. It is the payload chunk of the format being read — "data" for WAVE,
	// "movi" for AVI — and every other chunk keeps its declared size, so an
	// empty chunk ahead of the payload cannot swallow the stream. The allowance
	// applies only when the container header is itself unsized, since an
	// encoder that could not patch one could not patch the other; within a
	// sized container a zero size stays an empty chunk and an all-ones size is
	// an error. Left zero, an all-ones size is always rejected.
	AllowUnsizedChunk FourCC

	// IgnoreSize disregards the size the container header declares, leaving
	// the chunks bounded by their own sizes and the end of the stream. An
	// encoder that streams to a file and never patches it can leave sizes that
	// are neither placeholders nor right; with this set such a file reads as
	// far as its bytes go, and the AllowUnsizedChunk chunk may be unsized
	// whatever the header says.
	IgnoreSize bool

	// RF64 reports that the container is the 64-bit form of RIFF. Its header
	// size is a placeholder, so the container reads as unsized; the real sizes
	// are in its ds64 chunk, which comes first and is the caller's to read.
	RF64 bool

	// FormType is the container's form type, e.g. "WAVE".
	FormType FourCC

	// Size is the size declared by the container header, covering the form type
	// and every chunk that follows it.
	Size uint32

	r         io.Reader
	remaining int64 // bytes left in the container for chunks, -1 when unknown
	cur       *Chunk
	chunkRem  int64 // bytes left in the current chunk, -1 when unsized
	pad       int64 // pad bytes to discard after the current chunk
	gen       uint64
}

// New reads the container header, RIFF or RF64, and returns a Reader
// positioned at the first chunk.
func New(r io.Reader) (*Reader, error) {
	var buf [12]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("riff: short header: %w", ErrNotRIFF)
		}
		return nil, err
	}
	rf64 := string(buf[:4]) == "RF64"
	if !rf64 && string(buf[:4]) != "RIFF" {
		return nil, ErrNotRIFF
	}
	size := binary.LittleEndian.Uint32(buf[4:8])
	rr := &Reader{r: r, Size: size, RF64: rf64, remaining: -1}
	// A zero or all-ones size means the writer never patched it; the chunks
	// themselves then bound the container. RF64 states its size elsewhere.
	if !rf64 && size != 0 && size != unsizedSize {
		if size < 4 {
			return nil, fmt.Errorf("riff: header size %d is smaller than the form type: %w", size, ErrBadChunkSize)
		}
		rr.remaining = int64(size) - 4
	}
	copy(rr.FormType[:], buf[8:12])
	return rr, nil
}

// Next advances to the next chunk, discarding whatever is left of the current
// one. It returns io.EOF at the end of the container. The returned Chunk
// becomes stale as soon as Next is called again.
func (r *Reader) Next() (*Chunk, error) {
	// A Reader over a LIST chunk is only valid while that chunk is.
	if parent, ok := r.r.(*Chunk); ok {
		if err := parent.check(); err != nil {
			return nil, err
		}
	}
	if err := r.finish(); err != nil {
		return nil, err
	}
	r.gen++
	r.cur = nil

	if r.IgnoreSize {
		r.remaining = -1
	}
	if r.remaining == 0 {
		return nil, io.EOF
	}
	if r.remaining > 0 && r.remaining < 8 {
		return nil, fmt.Errorf("riff: %d bytes left, too few for a chunk header: %w", r.remaining, ErrBadChunkSize)
	}

	var hdr [8]byte
	n, err := io.ReadFull(r.r, hdr[:])
	if err != nil {
		if n == 0 && errors.Is(err, io.EOF) && r.remaining < 0 {
			return nil, io.EOF
		}
		return nil, unexpectedEOF(err)
	}
	if r.remaining >= 0 {
		r.remaining -= 8
	}

	c := &Chunk{Size: binary.LittleEndian.Uint32(hdr[4:8]), r: r, gen: r.gen}
	copy(c.ID[:], hdr[:4])

	// Only the selected payload chunk may run to EOF, and only when the container
	// itself declared no size: an encoder that could not patch the chunk size
	// could not patch the container size either. Anywhere else a zero size is an
	// ordinary empty chunk and the bytes after it belong to the chunks that
	// follow.
	allowed := r.AllowUnsizedChunk != FourCC{} && c.ID == r.AllowUnsizedChunk
	unsized := c.Size == unsizedSize || c.Size == 0
	switch {
	case unsized && allowed && r.remaining < 0:
		c.Unsized = true
		r.chunkRem = -1
		r.pad = 0
	case c.Size == unsizedSize && !allowed:
		return nil, fmt.Errorf("riff: chunk %s declares an unsized length; set AllowUnsizedChunk to accept it: %w", c.ID, ErrBadChunkSize)
	case c.Size == unsizedSize:
		return nil, fmt.Errorf("riff: chunk %s declares an unsized length inside a container of %d bytes: %w", c.ID, r.Size, ErrBadChunkSize)
	default:
		r.chunkRem = int64(c.Size)
		r.pad = int64(c.Size & 1)
		if r.remaining >= 0 {
			if r.chunkRem > r.remaining {
				return nil, fmt.Errorf("riff: chunk %s declares %d bytes with %d left in the container: %w", c.ID, c.Size, r.remaining, ErrBadChunkSize)
			}
			r.remaining -= r.chunkRem
			// The container size may or may not cover a trailing pad byte.
			if r.pad > r.remaining {
				r.pad = r.remaining
			}
			r.remaining -= r.pad
		}
	}

	r.cur = c
	return c, nil
}

// finish consumes the remainder of the current chunk and its pad byte.
func (r *Reader) finish() error {
	if r.cur == nil {
		return nil
	}
	if r.chunkRem < 0 {
		_, err := io.Copy(io.Discard, r.r)
		r.chunkRem = 0
		return err
	}
	if r.chunkRem > 0 {
		if _, err := io.CopyN(io.Discard, r.r, r.chunkRem); err != nil {
			return unexpectedEOF(err)
		}
		r.chunkRem = 0
	}
	if r.pad > 0 {
		// A missing final pad byte is common enough to tolerate. The next
		// header read reports the end of the stream.
		if _, err := io.CopyN(io.Discard, r.r, r.pad); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		r.pad = 0
	}
	return nil
}

// Chunk is a single RIFF chunk. It reads the chunk's data, bounded to Size,
// and is valid only until the parent Reader advances.
type Chunk struct {
	// ID identifies the chunk, e.g. "fmt ".
	ID FourCC

	// Size is the declared size of the chunk data, excluding the pad byte. It
	// is meaningless when Unsized is set.
	Size uint32

	// Unsized reports that the chunk declared no usable size and reads to the
	// end of the stream.
	Unsized bool

	r   *Reader
	gen uint64
}

// Read implements io.Reader over the chunk's data. It returns ErrStaleChunk
// once the parent Reader has advanced, and io.ErrUnexpectedEOF when the stream
// ends before the declared size.
func (c *Chunk) Read(p []byte) (int, error) {
	if err := c.check(); err != nil {
		return 0, err
	}
	r := c.r
	if r.chunkRem == 0 {
		return 0, io.EOF
	}
	if r.chunkRem > 0 && int64(len(p)) > r.chunkRem {
		p = p[:r.chunkRem]
	}
	if len(p) == 0 {
		return 0, nil
	}
	n, err := r.r.Read(p)
	if r.chunkRem > 0 {
		r.chunkRem -= int64(n)
		if errors.Is(err, io.EOF) && r.chunkRem > 0 {
			err = io.ErrUnexpectedEOF
		}
	}
	return n, err
}

// Bytes reads the rest of the chunk into memory. Chunks larger than limit are
// rejected with ErrChunkTooLarge rather than allocated, since the size comes
// from the file.
func (c *Chunk) Bytes(limit int) ([]byte, error) {
	if err := c.check(); err != nil {
		return nil, err
	}
	if c.Unsized {
		buf, err := io.ReadAll(io.LimitReader(c, int64(limit)+1))
		if err != nil {
			return nil, err
		}
		if len(buf) > limit {
			return nil, fmt.Errorf("riff: chunk %s exceeds the %d byte limit: %w", c.ID, limit, ErrChunkTooLarge)
		}
		return buf, nil
	}
	n := c.r.chunkRem
	if n > int64(limit) {
		return nil, fmt.Errorf("riff: chunk %s is %d bytes, limit %d: %w", c.ID, n, limit, ErrChunkTooLarge)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c, buf); err != nil {
		return nil, unexpectedEOF(err)
	}
	return buf, nil
}

// List reads the chunk's form type and returns a Reader over its subchunks, for
// container chunks such as LIST and RIFF. The returned Reader is valid for as
// long as the chunk is.
func (c *Chunk) List() (*Reader, error) {
	if err := c.check(); err != nil {
		return nil, err
	}
	var ft [4]byte
	if _, err := io.ReadFull(c, ft[:]); err != nil {
		return nil, unexpectedEOF(err)
	}
	sub := &Reader{
		AllowUnsizedChunk: c.r.AllowUnsizedChunk,
		Size:              c.Size,
		r:                 c,
		remaining:         c.r.chunkRem,
	}
	copy(sub.FormType[:], ft[:])
	return sub, nil
}

// Skip discards the rest of the chunk. Next does this too; Skip is for reading
// the chunk data out of order.
func (c *Chunk) Skip() error {
	if err := c.check(); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, c)
	return err
}

func (c *Chunk) check() error {
	if c.r.gen != c.gen {
		return ErrStaleChunk
	}
	return nil
}

func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
