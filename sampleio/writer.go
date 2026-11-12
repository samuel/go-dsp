package sampleio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"

	"samuelks.com/dsp"
	"samuelks.com/dsp/internal/view"
	"samuelks.com/dsp/pcm"
)

// WriterOptions configures a Writer.
//
// Format is required, and Rate is required for a WAV. Channels zero selects 2
// when IQ is set and 1 otherwise, and IQ requires exactly 2. Container zero
// selects WAV; Raw writes the samples with no header.
type WriterOptions struct {
	Layout

	// ChannelMask is a WAV's speaker assignment, a Speaker bit per position
	// in channel order. Zero leaves it to the header: none for a plain one, and
	// the usual mono or stereo mask where the format needs an extensible one.
	ChannelMask uint32
}

// Writer is the mirror of Reader: it takes full-scale samples and owns
// interleaving, byte order, bit depth and the container. Not safe for
// concurrent use. Samples are buffered, so Close, or Flush for a live
// consumer, is what gets the last of them to the destination.
type Writer struct {
	dst    io.Writer
	closer io.Closer // the file Create opened; nil for anything else

	// seeker is the destination when the WAV sizes can be patched on Close,
	// and start the offset its header begins at.
	seeker io.WriteSeeker
	start  int64

	info   Info
	stride int // bytes per frame
	hdr    wavLayout

	buf []byte // encoded bytes not yet written, header first
	n   int

	work  any // a planar write's interleaved scratch, a []T
	views any // a planar write's plane headers, a [][]T

	data    int64 // data chunk bytes encoded so far
	clipped bool
	err     error // sticky: the first error from the destination
	closed  bool
}

// errClosed is what every call on a closed Writer returns.
var errClosed = errors.New("sampleio: the Writer is closed")

// rf64Limit is the largest RIFF size a plain WAV can state. A file past it
// becomes RF64. It is a variable so a test can cross it without writing 4 GiB.
var rf64Limit int64 = math.MaxUint32

// Create writes to the file at path, replacing it. "-" or "" writes to
// standard output, which Close leaves open.
//
// A regular file can be sought, so Close fills in the WAV sizes; standard
// output usually cannot, and a WAV written there carries the placeholder
// sizes a pipe reader expects.
func Create(path string, opt WriterOptions) (*Writer, error) {
	if path == "" || path == "-" {
		return NewWriter(os.Stdout, opt)
	}
	// Checked before the file is created, so a bad option leaves an existing
	// file untouched.
	if _, err := writerInfo(opt); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w, err := NewWriter(f, opt)
	if err != nil {
		_ = f.Close() // the option error is the one worth reporting
		return nil, err
	}
	w.closer = f
	return w, nil
}

// NewWriter writes to w. When w is an io.WriteSeeker that can seek, Close
// goes back to fill in the WAV sizes, and turns the file into RF64 if it has
// grown past 4 GiB; otherwise the sizes are left as the placeholders a
// streaming writer uses. Close does not close w.
func NewWriter(w io.Writer, opt WriterOptions) (*Writer, error) {
	info, err := writerInfo(opt)
	if err != nil {
		return nil, err
	}
	wr := &Writer{dst: w, info: info}
	wr.stride = info.Format.Width() * info.Channels
	if s, ok := w.(io.WriteSeeker); ok {
		// os.Stdout is an io.WriteSeeker whether or not it is a pipe; asking
		// where it is tells the two apart.
		if off, err := s.Seek(0, io.SeekCurrent); err == nil {
			wr.seeker, wr.start = s, off
		}
	}

	wr.buf = make([]byte, max(blockBytes/wr.stride*wr.stride, wr.stride)+wr.stride)
	if info.Container == WAV {
		var hdr []byte
		hdr, wr.hdr, err = wavHeaderBytes(info.Format, info.Channels, uint32(info.Rate), info.ChannelMask, wr.seeker != nil)
		if err != nil {
			return nil, err
		}
		if len(hdr) > len(wr.buf) {
			wr.buf = make([]byte, len(hdr)+len(wr.buf))
		}
		wr.n = copy(wr.buf, hdr)
	}
	return wr, nil
}

// writerInfo validates opt and returns the Info a Writer starts with.
func writerInfo(opt WriterOptions) (Info, error) {
	l := opt.Layout
	if !l.Format.Valid() {
		return Info{}, errors.New("sampleio: writing needs a sample format")
	}
	switch {
	case l.Channels < 0:
		return Info{}, fmt.Errorf("sampleio: %d channels", l.Channels)
	case l.Channels == 0 && l.IQ:
		l.Channels = 2
	case l.Channels == 0:
		l.Channels = 1
	case l.IQ && l.Channels != 2:
		return Info{}, fmt.Errorf("sampleio: an I/Q stream is written as one pair of channels, not %d", l.Channels)
	}
	if math.IsNaN(l.Rate) || math.IsInf(l.Rate, 0) || l.Rate < 0 {
		return Info{}, fmt.Errorf("sampleio: a rate of %g Hz", l.Rate)
	}

	switch l.Container {
	case DetectContainer:
		l.Container = WAV
	case WAV, Raw:
	default:
		return Info{}, fmt.Errorf("sampleio: %v is not a container", l.Container)
	}
	if l.Container == WAV {
		if err := wavWritable(l); err != nil {
			return Info{}, err
		}
	}
	info := Info{Layout: l, Bits: l.Format.Bits()}
	if l.Container == WAV {
		info.ChannelMask = opt.ChannelMask
	}
	return info, nil
}

// wavWritable reports why a layout cannot be written as a WAV, if it cannot.
func wavWritable(l Layout) error {
	switch l.Format {
	case pcm.U8, pcm.I16LE, pcm.I24LE, pcm.I32LE, pcm.F32LE, pcm.F64LE, pcm.ALaw, pcm.MuLaw:
	case pcm.I8:
		return errors.New("sampleio: a WAV holds 8-bit samples unsigned; write U8")
	default:
		return fmt.Errorf("sampleio: a WAV is little-endian and cannot hold %v", l.Format)
	}
	if l.Rate < 1 || l.Rate != math.Trunc(l.Rate) || l.Rate > math.MaxUint32 {
		return fmt.Errorf("sampleio: a WAV needs a whole-number rate in Hz below 2^32, not %g", l.Rate)
	}
	block := l.Channels * l.Format.Width()
	if l.Channels > math.MaxUint16 || block > math.MaxUint16 {
		return fmt.Errorf("sampleio: %d channels do not fit a WAV header", l.Channels)
	}
	if l.Rate*float64(block) > math.MaxUint32 {
		return fmt.Errorf("sampleio: %g Hz of %d-byte frames is more bytes per second than a WAV header holds", l.Rate, block)
	}
	return nil
}

// Info returns what the Writer is writing. Frames counts the frames handed to
// it so far, buffered ones included.
func (w *Writer) Info() Info {
	info := w.info
	if w.clipped {
		info.Notes = append(slices.Clip(info.Notes),
			"a sample lay outside full scale and was clipped to the format's range")
	}
	return info
}

// WriteSamples encodes src and returns how many samples it took. T is real for
// a one-channel stream and complex for an I/Q one; a stream of more channels is
// written with WritePlanar.
//
// The samples are encoded at the width they are handed over in. Once the
// destination has failed, every write returns that error.
func (w *Writer) WriteSamples[T dsp.Sample](src []T) (int, error) {
	if err := w.writable(isComplex[T]()); err != nil {
		return 0, err
	}
	if !w.info.IQ && w.info.Channels != 1 {
		return 0, fmt.Errorf("sampleio: this stream has %d channels; write them with WritePlanar", w.info.Channels)
	}
	return writeInterleaved(w, src, 1)
}

// WritePlanar interleaves one slice per channel, or a single complex slice for
// an I/Q stream, and returns how many frames it wrote. Every plane must be the
// same length; nothing is written otherwise.
func (w *Writer) WritePlanar[T dsp.Sample](src [][]T) (int, error) {
	if err := w.writable(isComplex[T]()); err != nil {
		return 0, err
	}
	planes := w.info.Channels
	if w.info.IQ {
		planes = 1
	}
	if len(src) != planes {
		return 0, fmt.Errorf("sampleio: %d planes for a stream that takes %d", len(src), planes)
	}
	frames := len(src[0])
	for c, p := range src[1:] {
		if len(p) != frames {
			return 0, fmt.Errorf("sampleio: plane %d holds %d frames and plane 0 holds %d", c+1, len(p), frames)
		}
	}
	if planes == 1 {
		return writeInterleaved(w, src[0], 1)
	}

	block := max(len(w.buf)/w.stride, 1)
	inter := scratch[T](&w.work, block*planes)
	heads := scratch[[]T](&w.views, planes)
	done := 0
	for done < frames {
		k := min(block, frames-done)
		for c, p := range src {
			heads[c] = p[done : done+k]
		}
		dsp.Interleave(inter, heads)
		n, err := writeInterleaved(w, inter[:k*planes], planes)
		done += n
		if err != nil {
			clear(heads)
			return done, err
		}
	}
	// The arrays behind these headers are the caller's.
	clear(heads)
	return done, nil
}

// writable reports whether a write of a real or complex width can go ahead.
func (w *Writer) writable(complexT bool) error {
	switch {
	case w.closed:
		return errClosed
	case w.err != nil:
		return w.err
	case complexT == w.info.IQ:
		return nil
	case w.info.IQ:
		return errors.New("sampleio: this stream is I/Q; write complex64 or complex128")
	default:
		return errors.New("sampleio: this stream is real; write float32 or float64")
	}
}

// writeInterleaved encodes src, perFrame elements to a frame, into the buffer a
// block at a time, writing the buffer out whenever it fills. It returns the
// frames taken.
func writeInterleaved[T dsp.Sample](w *Writer, src []T, perFrame int) (int, error) {
	frames := len(src) / perFrame
	done := 0
	for done < frames {
		k := min(frames-done, (len(w.buf)-w.n)/w.stride)
		if k == 0 {
			if err := w.flushBuf(); err != nil {
				return done, err
			}
			continue
		}
		blk := src[done*perFrame : (done+k)*perFrame]
		noteClip(w, blk)
		pcm.Encode(w.buf[w.n:w.n+k*w.stride], blk, w.info.Format)
		w.n += k * w.stride
		w.data += int64(k * w.stride)
		w.info.Frames += int64(k)
		done += k
	}
	return done, nil
}

// noteClip records an integer sample outside full scale, which pcm.Encode
// clips without saying so. A float format holds any value, so it never clips.
func noteClip[T dsp.Sample](w *Writer, x []T) {
	if w.clipped || w.info.Format.Float() || len(x) == 0 {
		return
	}
	limit := 1.0
	if w.info.Format == pcm.MuLaw {
		limit = muLawLimit
	}
	switch v := any(x).(type) {
	case []float32:
		w.clipped = pastLimit(v, float32(limit))
	case []float64:
		w.clipped = pastLimit(v, limit)
	case []complex64:
		w.clipped = pastLimit(view.C64ToF32(v), float32(limit))
	case []complex128:
		w.clipped = pastLimit(view.C128ToF64(v), limit)
	}
}

// muLawLimit is the upper edge of mu-law's top step, half a step above its
// level: a sample past it is clipped rather than quantized. A-law's top step
// reaches full scale, so it clips where an integer format does.
const muLawLimit = (32124 + 512) / 32768.0

func pastLimit[F dsp.Float](x []F, limit F) bool {
	return dsp.VMax(x) > limit || dsp.VMin(x) < -limit
}

// flushBuf writes the buffer out, keeping the first error for every call
// after it.
func (w *Writer) flushBuf() error {
	if w.err != nil {
		return w.err
	}
	if w.n == 0 {
		return nil
	}
	m, err := w.dst.Write(w.buf[:w.n])
	if err == nil && m < w.n {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
		return err
	}
	w.n = 0
	return nil
}

// Flush writes everything buffered to the destination, for a consumer reading
// the stream live. The WAV sizes are not filled in until Close.
func (w *Writer) Flush() error {
	if w.closed {
		return errClosed
	}
	return w.flushBuf()
}

// Close writes everything buffered and, when the destination can seek, pads
// an odd-length data chunk and fills in the WAV sizes. It closes a file Create
// opened. Standard output, and any destination handed to NewWriter, stays
// open.
func (w *Writer) Close() error {
	if w.closed {
		return errClosed
	}
	w.closed = true

	err := w.err
	if err == nil && w.info.Container == WAV && w.seeker != nil && w.data%2 == 1 {
		// RIFF chunks are word-aligned; the pad byte is not counted in the
		// data size. A pipe's data chunk has no size and runs to the end of
		// the stream, so a pad byte there would read as one more sample.
		if w.n == len(w.buf) {
			err = w.flushBuf()
		}
		if err == nil {
			w.buf[w.n] = 0
			w.n++
		}
	}
	if err == nil {
		err = w.flushBuf()
	}
	if err == nil && w.info.Container == WAV && w.seeker != nil {
		err = w.patchSizes()
	}
	if w.closer != nil {
		if cerr := w.closer.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// patchSizes goes back over the header and writes the sizes a streaming header
// leaves as placeholders, turning the file into RF64 if a 32-bit size cannot
// hold them.
func (w *Writer) patchSizes() error {
	pad := w.data % 2
	end := int64(w.hdr.size) + w.data + pad
	riff := end - 8
	frames := w.info.Frames

	put := func(off int, b []byte) error {
		if _, err := w.seeker.Seek(w.start+int64(off), io.SeekStart); err != nil {
			return err
		}
		_, err := w.seeker.Write(b)
		return err
	}
	u32 := func(v int64) []byte {
		if v > math.MaxUint32 {
			v = math.MaxUint32
		}
		return binary.LittleEndian.AppendUint32(nil, uint32(v))
	}

	if riff > rf64Limit {
		ds64 := []byte("ds64")
		ds64 = binary.LittleEndian.AppendUint32(ds64, ds64Size)
		ds64 = binary.LittleEndian.AppendUint64(ds64, uint64(riff))
		ds64 = binary.LittleEndian.AppendUint64(ds64, uint64(w.data))
		ds64 = binary.LittleEndian.AppendUint64(ds64, uint64(frames))
		ds64 = binary.LittleEndian.AppendUint32(ds64, 0) // no table
		for _, p := range []struct {
			off int
			b   []byte
		}{
			{0, []byte("RF64")},
			{4, u32(math.MaxUint32)},
			{w.hdr.junk, ds64},
			{w.hdr.data, u32(math.MaxUint32)},
		} {
			if err := put(p.off, p.b); err != nil {
				return err
			}
		}
	} else {
		if err := put(4, u32(riff)); err != nil {
			return err
		}
		if err := put(w.hdr.data, u32(w.data)); err != nil {
			return err
		}
	}
	if w.hdr.fact >= 0 {
		if err := put(w.hdr.fact, u32(frames)); err != nil {
			return err
		}
	}
	_, err := w.seeker.Seek(w.start+end, io.SeekStart)
	return err
}
