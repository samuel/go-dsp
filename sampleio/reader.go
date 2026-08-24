package sampleio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/samuel/go-dsp/dsp"
)

// Channel selects which channel of a stream to read.
type Channel int

// MixChannels selects the mean of every channel rather than one of them.
const MixChannels Channel = -1

// blockBytes is the raw read size. Large enough that a read is not dominated by
// syscall overhead, small enough that a Reader costs nothing to hold.
const blockBytes = 1 << 16

// Info describes what a Reader found.
type Info struct {
	Format   Format
	Rate     float64
	Channels int // interleaved channels in the file, before selection

	// Frames is how many samples the Reader will produce, after Skip and Limit, or
	// -1 when that cannot be known without reading the whole stream.
	//
	// It is a length, not a guess: a header's claim is believed only once checked
	// against the size on disk. A pipe therefore reports -1 even when its header
	// states a size, and so does a file whose header overstates its data, with a
	// Note.
	Frames int64

	Complex   bool   // channel pairs are I/Q
	Container string // "wav" or "raw"
	Bits      int    // container bits per sample; 0 for a float format
	ValidBits int    // significant bits, when the header says fewer than Bits
	Codec     string // "pcm" or "float"

	// Notes records everything surprising but survivable: a rate override, a
	// header field not worth trusting, a float sample past full scale.
	Notes []string
}

// Duration returns the length in seconds, or 0 when Frames is unknown.
func (i Info) Duration() float64 {
	if i.Frames < 0 || i.Rate <= 0 {
		return 0
	}
	return float64(i.Frames) / i.Rate
}

// Options configures a Reader. The zero value reads a WAV as it finds it.
type Options struct {
	// Format is the encoding of a headerless stream. A container header wins
	// over it, and reading raw requires it.
	Format Format

	// Rate overrides the rate a header states, and is required for raw input.
	Rate float64

	// Channels is how many are interleaved in a headerless stream. Zero selects
	// 2 when IQ is set and 1 otherwise.
	Channels int

	// Channel selects which one to read. MixChannels takes the mean of all of
	// them. With IQ set it selects an I/Q pair rather than a single channel.
	Channel Channel

	// IQ reads channel pairs as interleaved I/Q, so the stream is complex and
	// half as many channels wide as it looks.
	IQ bool

	// Raw reads a file as a headerless stream even if it has a header, which is
	// how you look at a WAV whose header is wrong.
	Raw bool

	// Skip drops this many frames before the first sample, and Limit stops after
	// this many. Zero Limit reads to the end.
	Skip, Limit int64
}

// Reader hands over samples: one selected channel, or every channel at once
// into one slice each. Not safe for concurrent use. It reads in blocks, so a
// capture larger than memory costs no more than a short one.
type Reader struct {
	src    io.Reader
	closer io.Closer
	info   Info

	width  int     // bytes per element
	elems  int     // interleaved elements per frame
	stride int     // bytes per frame
	pick   Channel // channel, or I/Q pair index, or MixChannels
	iq     bool

	buf []byte // raw bytes, holding whole frames plus a partial tail
	n   int    // valid bytes in buf
	off int    // bytes of buf already decoded

	gath []byte    // one channel's bytes, gathered contiguously
	swap []byte    // byte-reversed samples, for a big-endian format
	work []float64 // decoded scratch: for mixing, for pairing I/Q, for a planar read

	// Reusable plane headers for a planar read, one field per width.
	// dsp.Deinterleave writes at the front of the planes it is handed, so a
	// block landing mid-plane re-points these instead of passing an offset.
	viewF64 [][]float64
	viewF32 [][]float32

	// narrow is ReadFloat32's staging, and deliberately not work: the decode
	// owns work and reallocates it, so holding it out as a destination would
	// hand the caller whatever the decode last put there.
	narrow []float64

	// overrange is set once a float sample outside full scale has been decoded;
	// see noteOverrange.
	overrange bool

	left int64 // frames still allowed by Limit; -1 for no limit
	eof  bool
}

// Open reads the file at path. "-" or "" reads standard input, whose length is
// not known ahead of time.
//
// A file is read as a WAV when it starts with one and Options.Raw is not set.
func Open(path string, opt Options) (*Reader, error) {
	if path == "" || path == "-" {
		r, err := newReader(os.Stdin, nil, opt, -1)
		if err != nil {
			return nil, err
		}
		return r, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// The size lets a raw stream report its length, which is what lets a caller
	// choose a frame count before reading anything.
	var size int64 = -1
	if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
		size = st.Size()
	}
	r, err := newReader(f, f, opt, size)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return r, nil
}

// NewReader reads a stream. The length is unknown, so Info().Frames is -1 unless
// a container header says otherwise.
func NewReader(r io.Reader, opt Options) (*Reader, error) {
	return newReader(r, nil, opt, -1)
}

func newReader(src io.Reader, closer io.Closer, opt Options, size int64) (*Reader, error) {
	if opt.Skip < 0 || opt.Limit < 0 {
		return nil, errors.New("sampleio: Skip and Limit cannot be negative")
	}

	r := &Reader{src: src, closer: closer, left: -1}
	r.info = Info{Container: "raw", Frames: -1}

	// A header, unless the caller said not to look for one.
	var dataBytes int64 = -1
	if !opt.Raw {
		h, err := sniffWAV(&r.src)
		if err != nil {
			return nil, err
		}
		if h != nil {
			r.info.Container = "wav"
			r.info.Format = h.format
			r.info.Rate = h.rate
			r.info.Channels = h.channels
			r.info.Bits = h.bits
			r.info.ValidBits = h.validBits
			r.info.Codec = h.codec
			r.info.Notes = h.notes
			dataBytes = h.dataBytes
			if size >= 0 && dataBytes > size {
				// A writer that streamed to a file and never went back to fix
				// the size. Trust the file, not the field.
				r.info.Notes = append(r.info.Notes, fmt.Sprintf(
					"the data chunk claims %d bytes, more than the file holds; reading to the end", dataBytes))
				dataBytes = -1
			}
		}
	}

	if r.info.Container == "raw" {
		if !opt.Format.Valid() {
			return nil, errors.New("sampleio: reading a headerless stream needs a sample format")
		}
		if opt.Rate <= 0 {
			return nil, errors.New("sampleio: reading a headerless stream needs a sample rate")
		}
		r.info.Format = opt.Format
		r.info.Rate = opt.Rate
		r.info.Channels = opt.Channels
		if r.info.Channels == 0 {
			r.info.Channels = 1
			if opt.IQ {
				r.info.Channels = 2
			}
		}
		_, _, _, bits := opt.Format.info()
		r.info.Bits = bits
		r.info.Codec = "pcm"
		if bits == 0 {
			r.info.Codec = "float"
		}
	} else {
		if opt.Rate > 0 && opt.Rate != r.info.Rate {
			r.info.Notes = append(r.info.Notes, fmt.Sprintf(
				"the header says %g Hz; reading it as %g Hz", r.info.Rate, opt.Rate))
			r.info.Rate = opt.Rate
		}
		if opt.Channels > 0 && opt.Channels != r.info.Channels {
			return nil, fmt.Errorf("sampleio: the header says %d channels, not %d",
				r.info.Channels, opt.Channels)
		}
	}

	if r.info.Channels <= 0 {
		return nil, fmt.Errorf("sampleio: %d channels", r.info.Channels)
	}
	r.width = r.info.Format.Width()
	r.elems = r.info.Channels
	r.stride = r.width * r.elems
	r.iq = opt.IQ
	r.info.Complex = opt.IQ
	r.pick = opt.Channel

	if err := r.selectChannel(); err != nil {
		return nil, err
	}

	r.buf = make([]byte, blockBytes/r.stride*r.stride+r.stride)
	if opt.Skip > 0 {
		if err := r.skip(opt.Skip); err != nil {
			return nil, err
		}
	}

	// How many frames there are to read, before the limit.
	var total int64 = -1
	switch {
	case dataBytes >= 0:
		total = dataBytes / int64(r.stride)
	case r.info.Container == "raw" && size >= 0:
		total = size / int64(r.stride)
	}

	// The data chunk size bounds the read whether or not it is trustworthy as a
	// length: it says where the samples stop, which is what keeps a trailing
	// LIST or id3 chunk from being decoded as samples.
	r.left = -1
	if total >= 0 {
		r.left = max(0, total-opt.Skip)
	}
	if opt.Limit > 0 && (r.left < 0 || opt.Limit < r.left) {
		r.left = opt.Limit
	}

	// Reporting it as a length needs the size on disk to check it against. A
	// streaming writer fills that field before it knows the answer, and what it
	// puts is not always the placeholder -- sox writes 0x3ffff800 down a pipe,
	// 6.8 hours at 44.1 kHz -- so an unverifiable length is unknown rather than
	// a guess.
	r.info.Frames = -1
	if size >= 0 {
		r.info.Frames = r.left
	} else if opt.Limit > 0 {
		// A limit is the caller's own number, so it needs no checking.
		r.info.Frames = opt.Limit
	}
	return r, nil
}

// selectChannel validates the channel choice against the stream's channel layout.
func (r *Reader) selectChannel() error {
	if r.iq {
		if r.pick == MixChannels {
			return errors.New("sampleio: an I/Q stream cannot be mixed down; its channels are not alternatives")
		}
		if r.elems%2 != 0 {
			return fmt.Errorf("sampleio: an I/Q stream needs channels in pairs, and this one has %d", r.elems)
		}
		if pairs := r.elems / 2; r.pick < 0 || int(r.pick) >= pairs {
			return fmt.Errorf("sampleio: I/Q pair %d of %d", r.pick, pairs)
		}
		return nil
	}
	if r.pick == MixChannels {
		return nil
	}
	if r.pick < 0 || int(r.pick) >= r.elems {
		return fmt.Errorf("sampleio: channel %d of %d", r.pick, r.elems)
	}
	return nil
}

// noteOverrange records a float sample outside full scale, which the package
// comment promises a Note for.
//
// Only a float format can have one: an integer sample is divided by its own
// full scale, so it lands in [-1, 1) by construction. It matters because every
// decibel downstream is relative to 1.0, and a float file need not respect it:
// a peak of +2.0 reads as +6 dBFS, clipping that is not there.
func (r *Reader) noteOverrange(x []float64) {
	if r.overrange || r.info.Codec != "float" {
		return
	}
	for _, v := range x {
		if v > 1 || v < -1 {
			r.overrange = true
			return
		}
	}
}

// Info returns what the Reader knows about the stream.
func (r *Reader) Info() Info {
	info := r.info
	if r.overrange {
		// Appended here rather than stored, so the note appears as soon as the
		// sample that earned it has been read.
		info.Notes = append(slices.Clip(info.Notes),
			"a float sample lies outside full scale, so levels are relative to a range the container does not enforce")
	}
	return info
}

// Close closes the underlying file, if there is one.
func (r *Reader) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

// ReadFloat64 fills dst with samples of the selected channel and returns how
// many it wrote. It returns io.EOF only when it wrote nothing.
//
// It is not io.Reader: the unit is a sample of one channel rather than a byte,
// so the count means something different. The width is in the name because
// every read carries one -- ReadFloat32, ReadComplex128 and the two planar
// forms -- so a caller picks a method rather than recalling which one the
// unqualified name meant.
//
// Reading a stream opened with IQ is an error: the samples are complex, and
// handing back only the real part would be worse than saying so.
func (r *Reader) ReadFloat64(dst []float64) (int, error) {
	if r.iq {
		return 0, errors.New("sampleio: this stream is I/Q; use ReadComplex128")
	}
	total := 0
	for total < len(dst) {
		frames, err := r.ready(len(dst) - total)
		if err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, err
		}
		r.decodeReal(dst[total:total+frames], frames)
		r.advance(frames)
		total += frames
	}
	return total, nil
}

// ReadComplex128 fills dst with I/Q samples of the selected pair.
func (r *Reader) ReadComplex128(dst []complex128) (int, error) {
	if !r.iq {
		return 0, errors.New("sampleio: this stream is real; use ReadFloat64")
	}
	total := 0
	for total < len(dst) {
		frames, err := r.ready(len(dst) - total)
		if err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, err
		}
		r.decodeComplex(dst[total:total+frames], frames)
		r.advance(frames)
		total += frames
	}
	return total, nil
}

// ReadFloat32 is ReadFloat64 at the width the filters in dsp take. It stages a
// block at a time, so reading a whole file in one call does not also allocate a
// float64 copy of it.
func (r *Reader) ReadFloat32(dst []float32) (int, error) {
	total := 0
	for total < len(dst) {
		buf := r.staging(len(dst) - total)
		n, err := r.ReadFloat64(buf)
		for i, v := range buf[:n] {
			dst[total+i] = float32(v)
		}
		total += n
		if err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, err
		}
		if n < len(buf) {
			// A shortfall is reported by coming back short; the call after this
			// one is the one that says EOF.
			return total, nil
		}
	}
	return total, nil
}

// ReadFloat64Planar fills one slice per channel with that channel's samples and returns
// how many frames it wrote to each. It returns io.EOF only when it wrote nothing.
//
// dst holds one plane per channel of the file, in file order, and must hold
// exactly as many as Info().Channels reports. Options.Channel is not consulted:
// this is the whole frame rather than a selection, and a Reader opened for a
// mixdown is an error rather than a silent choice between the two.
//
// It is one pass over the bytes for every channel at once, so it is several
// times faster than a Reader per channel, which reads the file once per channel.
//
// Planes may differ in length; the shortest is what gets read, and the frames
// that do not fit are not consumed either. The planes must not overlap.
func (r *Reader) ReadFloat64Planar(dst [][]float64) (int, error) {
	if err := r.planarOK(len(dst)); err != nil {
		return 0, err
	}
	if r.elems == 1 {
		// One channel is already contiguous, so this is exactly ReadFloat64:
		// a decode straight into the caller's slice, with no scatter.
		return r.ReadFloat64(dst[0])
	}
	r.viewF64 = ensure(r.viewF64, len(dst))
	return readPlanar(r, dst, r.viewF64)
}

// ReadFloat32Planar is ReadFloat64Planar at the width the filters in dsp take.
func (r *Reader) ReadFloat32Planar(dst [][]float32) (int, error) {
	if err := r.planarOK(len(dst)); err != nil {
		return 0, err
	}
	if r.elems == 1 {
		return r.ReadFloat32(dst[0])
	}
	r.viewF32 = ensure(r.viewF32, len(dst))
	return readPlanar(r, dst, r.viewF32)
}

// ensure returns s with room for n elements, reallocating only when it is short.
func ensure[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

// staging returns a float64 buffer of up to want frames, bounded by the frames
// one raw block holds so that it cannot grow with the caller's request.
func (r *Reader) staging(want int) []float64 {
	n := min(want, len(r.buf)/r.stride)
	if cap(r.narrow) < n {
		r.narrow = make([]float64, n)
	}
	return r.narrow[:n]
}

// ready makes sure at least one whole frame is decodable and returns how many
// are, up to want.
func (r *Reader) ready(want int) (int, error) {
	for {
		frames := (r.n - r.off) / r.stride
		if r.left >= 0 && int64(frames) > r.left {
			frames = int(r.left)
		}
		if frames > 0 {
			return min(frames, want), nil
		}
		if r.left == 0 {
			return 0, io.EOF
		}
		if r.eof {
			return 0, io.EOF
		}
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
}

// fill slides the undecoded tail to the front and reads more on top of it.
func (r *Reader) fill() error {
	if r.off > 0 {
		r.n = copy(r.buf, r.buf[r.off:r.n])
		r.off = 0
	}
	// io.Reader permits (0, nil), and a caller is told to treat it as nothing
	// happened -- but a source that only ever returns it would spin here
	// forever. The limit is what io.ReadFull's own loop uses the error for:
	// enough attempts that a slow source is not cut off, few enough that a
	// broken one is reported instead of hanging.
	const maxEmptyReads = 100
	empty := 0
	for r.n < r.stride {
		m, err := r.src.Read(r.buf[r.n:])
		r.n += m
		if err != nil {
			if errors.Is(err, io.EOF) {
				r.eof = true
				// A trailing partial frame is a truncated file, not a sample.
				return nil
			}
			return err
		}
		if m == 0 {
			if empty++; empty >= maxEmptyReads {
				return io.ErrNoProgress
			}
			continue
		}
		empty = 0
	}
	// Top the buffer up opportunistically, but never block for it.
	if r.n < len(r.buf) {
		m, err := r.src.Read(r.buf[r.n:])
		r.n += m
		if err != nil && errors.Is(err, io.EOF) {
			r.eof = true
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (r *Reader) advance(frames int) {
	r.off += frames * r.stride
	if r.left > 0 {
		r.left -= int64(frames)
	}
}

// decodeReal writes frames samples of the selected channel into dst.
func (r *Reader) decodeReal(dst []float64, frames int) {
	src := r.buf[r.off:]
	f := r.info.Format

	switch {
	case r.pick == MixChannels:
		w := r.decodeFrames(frames)
		inv := 1 / float64(r.elems)
		for i := range dst {
			var sum float64
			for c := range r.elems {
				sum += w[i*r.elems+c]
			}
			dst[i] = sum * inv
		}

	case r.elems == 1:
		// The common case: mono, so the samples are already contiguous and the
		// decode is one call over the whole run.
		f.decode(dst, src, &r.swap)

	default:
		f.decode(dst, r.gather(src, frames, int(r.pick), 1), &r.swap)
	}
	r.noteOverrange(dst)
}

// decodeFrames decodes whole frames of every channel and returns the
// interleaved run.
//
// It is what a mixdown and a planar read share, and why neither gathers. The
// exactly-sized run bounds the decode: src runs to the end of buf and
// Format.decode stops at the shorter of its two slices. It also normalizes
// every channel in one vectorized pass.
func (r *Reader) decodeFrames(frames int) []float64 {
	n := frames * r.elems
	if cap(r.work) < n {
		r.work = make([]float64, n)
	}
	w := r.work[:n]
	r.info.Format.decode(w, r.buf[r.off:], &r.swap)
	r.noteOverrange(w)
	return w
}

// planarOK reports whether a planar read makes sense on this Reader, and
// whether the caller brought the right number of planes. MixChannels is
// rejected rather than ignored: a mixdown has no planar form.
func (r *Reader) planarOK(planes int) error {
	if r.iq {
		return errors.New("sampleio: this stream is I/Q; use ReadComplex128")
	}
	if r.pick == MixChannels {
		return errors.New("sampleio: this Reader mixes its channels down, and a mixdown has no planar form")
	}
	if planes != r.elems {
		return fmt.Errorf("sampleio: %d planes for a %d-channel stream", planes, r.elems)
	}
	return nil
}

// readPlanar is the body of both planar reads. It is a function because a method
// cannot take a type parameter, and it takes the plane view as an argument
// because each width has its own headers on the Reader.
func readPlanar[T dsp.Float](r *Reader, dst, view [][]T) (int, error) {
	want := planeFrames(dst)
	total := 0
	for total < want {
		frames, err := r.ready(want - total)
		if err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, err
		}
		for c, p := range dst {
			view[c] = p[total : total+frames]
		}
		deinterleave(view, r.decodeFrames(frames))
		r.advance(frames)
		total += frames
	}
	// The arrays behind these headers are the caller's; leaving them pointed at
	// the planes would keep them alive as long as the Reader is held.
	clear(view)
	return total, nil
}

// planeFrames is how many frames a planar read can write: as many as the
// shortest plane holds. planarOK has already checked there is at least one.
func planeFrames[T dsp.Float](dst [][]T) int {
	n := len(dst[0])
	for _, p := range dst[1:] {
		n = min(n, len(p))
	}
	return n
}

// deinterleave splits a decoded interleaved run into one plane per channel.
//
// At float64 that is dsp.Deinterleave. At float32 it cannot be: Deinterleave is
// a permutation and deliberately does not convert width, so the narrowing read
// fuses the two here instead of allocating float64 planes to narrow in a second
// pass. The type switch is an optimization, not a correctness fork -- the loop
// below is right at float64 too, only slower.
func deinterleave[T dsp.Float](dst [][]T, w []float64) {
	if d, ok := any(dst).([][]float64); ok {
		dsp.Deinterleave(d, w)
		return
	}
	channels := len(dst)
	frames := len(w) / channels
	for _, p := range dst {
		frames = min(frames, len(p))
	}
	if frames <= 0 {
		return
	}
	for c, p := range dst {
		p = p[:frames]
		src := w[c:]
		for i := range p {
			p[i] = T(src[i*channels])
		}
	}
}

// decodeComplex writes frames I/Q samples of the selected pair into dst.
func (r *Reader) decodeComplex(dst []complex128, frames int) {
	src := r.buf[r.off:]
	f := r.info.Format
	if r.elems != 2 {
		src = r.gather(src, frames, int(r.pick)*2, 2)
	}
	if cap(r.work) < 2*frames {
		r.work = make([]float64, 2*frames)
	}
	w := r.work[:2*frames]
	f.decode(w, src, &r.swap)
	r.noteOverrange(w)
	for i := range dst {
		dst[i] = complex(w[i*2], w[i*2+1])
	}
}

// gather copies count elements per frame, starting at element first, into one
// contiguous run.
//
// It is a byte shuffle rather than a strided decode so the twelve decode loops
// only ever see contiguous input; the mono case and a planar read skip it. It
// is the slow way round for more than one channel, because the copy length is
// not a constant, so each frame costs a call rather than a move.
func (r *Reader) gather(src []byte, frames, first, count int) []byte {
	need := frames * count * r.width
	if cap(r.gath) < need {
		r.gath = make([]byte, need)
	}
	out := r.gath[:need]
	run := count * r.width
	for i := range frames {
		copy(out[i*run:], src[i*r.stride+first*r.width:][:run])
	}
	return out
}

// skip drops frames before the first sample. A seekable source jumps; anything
// else reads and discards, which is what makes skipping work on a pipe.
func (r *Reader) skip(frames int64) error {
	want := frames * int64(r.stride)
	if s, ok := r.src.(io.Seeker); ok {
		if _, err := s.Seek(want, io.SeekCurrent); err == nil {
			return nil
		}
		// A source that claims to seek but will not: read past it instead.
	}
	if n, err := io.CopyN(io.Discard, r.src, want); err != nil {
		if errors.Is(err, io.EOF) {
			// Skipping past the end leaves an empty reader rather than failing.
			r.eof = true
			r.info.Notes = append(r.info.Notes, fmt.Sprintf(
				"asked to skip %d frames but the stream held %d", frames, n/int64(r.stride)))
			return nil
		}
		return err
	}
	return nil
}
