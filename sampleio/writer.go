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

// Writer encodes full-scale samples and writes them in the format, channel
// count and container of its Layout. Samples are buffered, so the last of them
// reach the destination on Close, or on Flush for a consumer reading the
// stream live. It is not safe for concurrent use.
type Writer struct {
	dst    io.Writer
	closer io.Closer // the file Create opened; nil for anything else

	// seeker is the destination when it can seek, which is what lets Close
	// fill in the WAV sizes. start is the offset of the header.
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

// errClosed is returned by every call on a closed Writer.
var errClosed = errors.New("sampleio: the Writer is closed")

// rf64Limit is the largest size a plain WAV header can state. A larger file
// is written as RF64. It is a variable so a test can lower it.
var rf64Limit int64 = math.MaxUint32

// Create creates the file at path, replacing any file already there, and
// writes to it. "-" or "" writes to standard output, which Close leaves open.
//
// l.Format is required, and l.Rate is required for a WAV. A zero Container
// writes a WAV; Raw writes the samples with no header.
//
// Close fills in the sizes in the header of a WAV written to a regular file.
// Standard output usually cannot seek, so a WAV written there keeps the
// placeholder sizes that tell a reader to read to the end of the stream.
func Create(path string, l Layout) (*Writer, error) {
	if path == "" || path == "-" {
		return NewWriter(os.Stdout, l)
	}
	// Check the layout before creating the file, so a bad one does not
	// truncate a file that is already there.
	if _, err := writerInfo(l); err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w, err := NewWriter(f, l)
	if err != nil {
		_ = f.Close() // the layout error is the one to report
		return nil, err
	}
	w.closer = f
	return w, nil
}

// NewWriter writes to w, with l interpreted as it is by Create. When w is an
// io.WriteSeeker that can seek, Close goes back and fills in the sizes in a
// WAV header, and makes the file RF64 if it has grown past 4 GiB. Otherwise
// the header keeps its placeholder sizes. Close does not close w.
func NewWriter(w io.Writer, l Layout) (*Writer, error) {
	info, err := writerInfo(l)
	if err != nil {
		return nil, err
	}
	wr := &Writer{dst: w, info: info}
	wr.stride = info.Format.Width() * info.Channels
	if s, ok := w.(io.WriteSeeker); ok {
		// os.Stdout always has a Seek method, but it fails on a pipe, so try
		// it.
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

// writerInfo validates l, fills in its defaults and returns the Info a Writer
// starts with.
func writerInfo(l Layout) (Info, error) {
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
	if l.Container != WAV {
		l.ChannelMask = 0
	}
	return Info{Layout: l, Bits: l.Format.Bits()}, nil
}

// wavWritable returns an error when a WAV cannot hold the layout.
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

// Info returns the layout being written. Frames counts the frames written so
// far, including those still buffered.
func (w *Writer) Info() Info {
	info := w.info
	if w.clipped {
		info.Notes = append(slices.Clip(info.Notes),
			"a sample lay outside full scale and was clipped to the format's range")
	}
	return info
}

// WriteSamples writes src to a stream that holds one signal: a single channel,
// or a single I/Q pair. It returns how many samples it wrote. A stream with
// more channels is written with WritePlanar.
//
// T must be float32 or float64 for a real stream and complex64 or complex128
// for an I/Q stream. Once a write to the destination has failed, every later
// write returns the same error.
func (w *Writer) WriteSamples[T dsp.Sample](src []T) (int, error) {
	if err := w.writable(isComplex[T]()); err != nil {
		return 0, err
	}
	if !w.info.IQ && w.info.Channels != 1 {
		return 0, fmt.Errorf("sampleio: this stream has %d channels; write them with WritePlanar", w.info.Channels)
	}
	return writeInterleaved(w, src, 1)
}

// WritePlanar writes every channel at once, each from its own slice, and
// returns how many frames it wrote. src holds one slice, or plane, per channel
// in file order, or a single complex plane for an I/Q stream. Every plane must
// be the same length; if they differ, nothing is written.
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
	// Drop the references to the caller's planes so the Writer does not keep
	// them alive.
	clear(heads)
	return done, nil
}

// writable returns an error when the Writer is closed or has failed, or when
// the samples are real and the stream I/Q, or the reverse.
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

// writeInterleaved encodes src into the buffer, writing the buffer out each
// time it fills. src holds perFrame elements per frame. It returns the number
// of frames encoded.
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

// noteClip records that a sample outside full scale was written to an integer
// format, so Info can add a Note about it. pcm.Encode clips such a sample. A
// float format holds any value, so nothing is clipped.
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

// muLawLimit is the largest magnitude mu-law can represent: the upper edge of
// its top step, half a step above the top level. A sample past it is clipped.
// A-law's top step reaches full scale, so it clips at 1 like an integer
// format.
const muLawLimit = (32124 + 512) / 32768.0

func pastLimit[F dsp.Float](x []F, limit F) bool {
	return dsp.VMax(x) > limit || dsp.VMin(x) < -limit
}

// flushBuf writes the buffer to the destination. The first error is kept and
// returned by every later call.
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

// Flush writes the buffered samples to the destination, for a consumer
// reading the stream live. The sizes in a WAV header are still filled in only
// by Close.
func (w *Writer) Flush() error {
	if w.closed {
		return errClosed
	}
	return w.flushBuf()
}

// Close writes the buffered samples. When the destination can seek, it also
// fills in the sizes in a WAV header and pads the data to an even length. It
// closes a file that Create opened, and leaves standard output and any
// destination given to NewWriter open.
func (w *Writer) Close() error {
	if w.closed {
		return errClosed
	}
	w.closed = true

	err := w.err
	if err == nil && w.info.Container == WAV && w.seeker != nil && w.data%2 == 1 {
		// A RIFF chunk of odd length is followed by a pad byte, which its size
		// does not count. A pipe gets none: its data chunk has no size and
		// runs to the end of the stream, so a reader would take the pad byte
		// for a sample.
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

// patchSizes replaces the placeholder sizes in the header with the real ones.
// When they do not fit in 32 bits it rewrites the header as RF64.
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
