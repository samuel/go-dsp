package sampleio

import (
	"bufio"
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

// ErrNotSeekable is returned by SeekFrame when the source cannot seek.
var ErrNotSeekable = errors.New("sampleio: the source cannot seek")

// blockBytes is how many bytes a Reader asks its source for at a time: large
// enough that system calls are not the main cost, small enough to be cheap to
// hold.
const blockBytes = 1 << 16

// Info describes a stream. A Reader's Info holds the layout it is reading:
// the header's for a WAV, with any Rate override applied, and Container set to
// WAV or Raw. A Writer's Info holds the layout it is writing.
type Info struct {
	Layout

	// Frames is how many frames the stream holds, or -1 when that is not known
	// without reading it to the end. A frame is one sample of every channel.
	//
	// A WAV header's size is reported only when it can be checked against the
	// size of the file. A pipe therefore reports -1 even when its header
	// states a size, and so does a file whose header claims more data than the
	// file holds, with a Note.
	//
	// For a Writer it is the number of frames written so far.
	Frames int64

	Bits      int // container bits per sample; 0 for a float format, 8 for G.711
	ValidBits int // significant bits, when the header says fewer than Bits

	// Notes lists anything odd that did not stop the read or write: a rate
	// override, a header field that was ignored, a float sample past full
	// scale, a sample that was clipped.
	Notes []string
}

// Duration returns the length in seconds, or 0 when Frames is unknown.
func (i Info) Duration() float64 {
	if i.Frames < 0 || i.Rate <= 0 {
		return 0
	}
	return float64(i.Frames) / i.Rate
}

// Reader decodes a stream into full-scale samples. It reads the source in
// blocks, so a long capture uses no more memory than a short one. It is not
// safe for concurrent use.
type Reader struct {
	src    io.Reader
	closer io.Closer
	info   Info
	chunks []Chunk

	// seeker is the source when it can seek; src may be a buffer in front of
	// it. dataStart is the offset of the first sample byte.
	seeker    io.ReadSeeker
	dataStart int64

	width  int // bytes per element
	elems  int // interleaved elements per frame
	stride int // bytes per frame
	iq     bool

	buf []byte // raw bytes: whole frames, then possibly part of one
	n   int    // valid bytes in buf
	off int    // bytes of buf already decoded

	// work holds a decoded interleaved block during a planar read. It is a
	// []T at the width of the last such read.
	work any

	// overrange is set once a float sample outside full scale has been
	// decoded; see noteOverrange.
	overrange bool

	total int64 // frames the stream holds, or -1 when it has no stated end
	pos   int64 // frames consumed so far
	eof   bool  // the source has returned io.EOF
}

// Open opens the file at path for reading. "-" or "" reads standard input.
//
// A file that starts with a WAV header is read as a WAV unless l.Container
// says otherwise. Anything else is a headerless stream, and l must give its
// format and rate. The zero Layout reads a WAV as it is.
func Open(path string, l Layout) (*Reader, error) {
	if path == "" || path == "-" {
		return newReader(os.Stdin, nil, l, -1)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// The file size is what lets Info report a length before anything is read.
	var size int64 = -1
	if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
		size = st.Size()
	}
	r, err := newReader(f, f, l, size)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return r, nil
}

// NewReader reads samples from r, with l interpreted as it is by Open. The
// length of r is not known, so Info().Frames is -1. Close does not close r.
func NewReader(r io.Reader, l Layout) (*Reader, error) {
	return newReader(r, nil, l, -1)
}

// newReader builds a Reader over src. size is the size of the file in bytes,
// or -1 when src is not a regular file.
func newReader(src io.Reader, closer io.Closer, l Layout, size int64) (*Reader, error) {
	if l.Container < DetectContainer || l.Container > Raw {
		return nil, fmt.Errorf("sampleio: %v is not a container", l.Container)
	}

	r := &Reader{src: src, closer: closer}
	r.info = Info{Container: Raw, Frames: -1}
	// A pipe has a Seek method that fails, so try it.
	if s, ok := src.(io.ReadSeeker); ok {
		if _, err := s.Seek(0, io.SeekCurrent); err == nil {
			r.seeker = s
		}
	}

	// Look for a WAV header unless the caller said the stream has none.
	var dataBytes int64 = -1
	if l.Container != Raw {
		h, err := sniffWAV(&r.src)
		if err != nil {
			return nil, err
		}
		if h == nil && l.Container == WAV {
			return nil, errors.New("sampleio: the stream does not start with a WAV header")
		}
		if h != nil {
			r.info.Container = WAV
			r.info.Format = h.format
			r.info.Rate = float64(h.rate)
			r.info.Channels = h.channels
			r.info.Bits = h.bits
			r.info.ValidBits = h.validBits
			r.info.ChannelMask = h.mask
			r.chunks = h.chunks
			r.info.Notes = h.notes
			dataBytes = h.dataBytes
			if size >= 0 && dataBytes > size {
				// The writer streamed to a file and never went back to fix the
				// size. The file size is the one to believe.
				r.info.Notes = append(r.info.Notes, fmt.Sprintf(
					"the data chunk claims %d bytes, more than the file holds; reading to the end", dataBytes))
				dataBytes = -1
			}
		}
	}

	if r.info.Container == Raw {
		if !l.Format.Valid() {
			return nil, errors.New("sampleio: reading a headerless stream needs a sample format")
		}
		if l.Rate <= 0 {
			return nil, errors.New("sampleio: reading a headerless stream needs a sample rate")
		}
		r.info.Format = l.Format
		r.info.Rate = l.Rate
		r.info.Channels = l.Channels
		if r.info.Channels == 0 {
			r.info.Channels = 1
			if l.IQ {
				r.info.Channels = 2
			}
		}
		r.info.Bits = l.Format.Bits()
		r.info.ChannelMask = l.ChannelMask
	} else {
		if l.Rate > 0 && l.Rate != r.info.Rate {
			r.info.Notes = append(r.info.Notes, fmt.Sprintf(
				"the header says %g Hz; reading it as %g Hz", r.info.Rate, l.Rate))
			r.info.Rate = l.Rate
		}
		if l.Channels > 0 && l.Channels != r.info.Channels {
			return nil, fmt.Errorf("sampleio: the header says %d channels, not %d",
				r.info.Channels, l.Channels)
		}
	}

	if r.info.Channels <= 0 {
		return nil, fmt.Errorf("sampleio: %d channels", r.info.Channels)
	}
	if l.IQ && r.info.Channels%2 != 0 {
		return nil, fmt.Errorf("sampleio: an I/Q stream needs channels in pairs, and this one has %d", r.info.Channels)
	}
	r.width = r.info.Format.Width()
	r.elems = r.info.Channels
	r.stride = r.width * r.elems
	r.iq = l.IQ
	r.info.IQ = l.IQ

	if r.seeker != nil {
		// The header was read through a buffer, so the source is further along
		// than the first sample by whatever the buffer still holds.
		at, err := r.seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		if br, ok := r.src.(*bufio.Reader); ok {
			at -= int64(br.Buffered())
		}
		r.dataStart = at
	}

	r.buf = make([]byte, blockBytes/r.stride*r.stride+r.stride)

	// The data chunk size always bounds the read, even when it is not trusted
	// as a length: it marks where the samples stop, so a LIST or id3 chunk
	// after them is not decoded as samples.
	r.total = -1
	switch {
	case dataBytes >= 0:
		r.total = dataBytes / int64(r.stride)
	case r.info.Container == Raw && size >= 0:
		r.total = size / int64(r.stride)
	}

	// The size is reported as a length only when there is a file size to check
	// it against. A writer streaming down a pipe fills the field in before it
	// knows the answer, and not always with the usual placeholder: sox writes
	// 0x3ffff800, which is 6.8 hours at 44.1 kHz.
	if size >= 0 {
		r.info.Frames = r.total
	}
	return r, nil
}

// noteOverrange records that a float sample outside full scale was read, so
// Info can add a Note about it.
//
// Only a float format can hold one: an integer sample is divided by its own
// full scale, so it is always in [-1, 1). The Note matters because levels are
// measured relative to 1.0, and a float file with a peak of 2.0 reads as
// +6 dBFS.
func noteOverrange[T dsp.Sample](r *Reader, x []T) {
	if r.overrange || !r.info.Format.Float() {
		return
	}
	switch v := any(x).(type) {
	case []float32:
		r.overrange = outsideFullScale(v)
	case []float64:
		r.overrange = outsideFullScale(v)
	case []complex64:
		r.overrange = outsideFullScale(view.C64ToF32(v))
	case []complex128:
		r.overrange = outsideFullScale(view.C128ToF64(v))
	}
}

func outsideFullScale[F dsp.Float](x []F) bool {
	for _, v := range x {
		if v > 1 || v < -1 {
			return true
		}
	}
	return false
}

// Info returns what the Reader knows about the stream.
func (r *Reader) Info() Info {
	info := r.info
	if r.overrange {
		// Added to the copy, so the stored notes are not changed.
		info.Notes = append(slices.Clip(info.Notes),
			"a float sample lies outside full scale, so levels are relative to a range the container does not enforce")
	}
	return info
}

// Chunks returns the chunks of a WAV that come before its samples, in file
// order, leaving out the format chunk. A chunk over 1 MiB, or one that would
// take the total past 4 MiB, is listed with nil Data. Chunks after the samples
// are not read.
func (r *Reader) Chunks() []Chunk { return r.chunks }

// SeekFrame moves the Reader so the next read starts at frame n, counted from
// the first frame of the stream. It returns ErrNotSeekable unless the source
// can seek, as a file from Open or an io.ReadSeeker given to NewReader can.
//
// When the stream states its length, n may be anything from 0 to that length.
// When it does not, any n from 0 up is accepted, and a read past the last
// frame returns io.EOF.
func (r *Reader) SeekFrame(n int64) error {
	if r.seeker == nil {
		return ErrNotSeekable
	}
	stride := int64(r.stride)
	if n < 0 || (r.total >= 0 && n > r.total) || n > (math.MaxInt64-r.dataStart)/stride {
		return fmt.Errorf("sampleio: frame %d is outside the stream", n)
	}
	return r.seekTo(n)
}

// seekTo moves the source to frame n and empties the buffers.
func (r *Reader) seekTo(n int64) error {
	if _, err := r.seeker.Seek(r.dataStart+n*int64(r.stride), io.SeekStart); err != nil {
		return err
	}
	if br, ok := r.src.(*bufio.Reader); ok {
		br.Reset(r.seeker)
	}
	r.n, r.off, r.eof = 0, 0, false
	r.pos = n
	return nil
}

// Skip moves forward n frames without decoding them and returns how many it
// skipped. It returns io.EOF when the stream ended before n frames.
//
// It works on any source: one that can seek is moved directly, and anything
// else, such as a pipe, is read and discarded.
func (r *Reader) Skip(n int64) (int64, error) {
	if n < 0 {
		return 0, errors.New("sampleio: Skip cannot move backward; use SeekFrame")
	}
	want := n
	if r.total >= 0 {
		want = min(want, r.total-r.pos)
	}

	// Frames already in the buffer cost nothing to drop.
	done := min(want, int64((r.n-r.off)/r.stride))
	r.advance(int(done))

	if done < want && r.seeker != nil {
		// Seeking past the end of a file succeeds, so find the end first and
		// stop there.
		if end, err := r.seeker.Seek(0, io.SeekEnd); err == nil {
			held := max((end-r.dataStart)/int64(r.stride)-r.pos, 0)
			k := min(want-done, held)
			if err := r.seekTo(r.pos + k); err != nil {
				return done, err
			}
			done += k
		}
	}

	// Read and discard the rest. After a seek this only finds the end.
	for done < want {
		frames, err := r.ready(int(min(want-done, math.MaxInt32)))
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return done, err
		}
		r.advance(frames)
		done += int64(frames)
	}
	if done < n {
		return done, io.EOF
	}
	return done, nil
}

// Close closes the file Open opened. A Reader from NewReader has nothing to
// close.
func (r *Reader) Close() error {
	if r.closer == nil {
		return nil
	}
	return r.closer.Close()
}

// ReadSamples fills dst from a stream that holds one signal: a single channel,
// or a single I/Q pair. It returns how many samples it read, and io.EOF only
// when it read none. A stream with more channels is read with ReadPlanar.
//
// The samples are decoded directly at the width of dst. T must be float32 or
// float64 for a real stream and complex64 or complex128 for an I/Q stream.
//
// It is named ReadSamples because it does not follow io.Reader's contract: it
// counts samples, not bytes.
func (r *Reader) ReadSamples[T dsp.Sample](dst []T) (int, error) {
	if err := r.checkKind(isComplex[T]()); err != nil {
		return 0, err
	}
	if planes, unit := r.planes(); planes != 1 {
		return 0, fmt.Errorf("sampleio: this stream has %d %s; read them with ReadPlanar", planes, unit)
	}
	return readOne(r, dst)
}

// ReadPlanar reads every channel at once, each into its own slice. It returns
// how many frames it read into each, and io.EOF only when it read none.
//
// dst holds one slice, or plane, per channel in file order. For an I/Q stream
// T is complex and there is one plane per I/Q pair. The number of planes must
// match the stream.
//
// A nil plane skips that channel: its samples are passed over and not stored.
// So reading only channel 1 of a stereo file is
//
//	r.ReadPlanar([][]float32{nil, buf})
//
// At least one plane must not be nil. Planes may differ in length; the read
// stops at the shortest, and frames past it stay in the stream for the next
// read. The planes must not overlap.
func (r *Reader) ReadPlanar[T dsp.Sample](dst [][]T) (int, error) {
	if err := r.checkKind(isComplex[T]()); err != nil {
		return 0, err
	}
	planes, unit := r.planes()
	if len(dst) != planes {
		return 0, fmt.Errorf("sampleio: %d planes for a stream of %d %s", len(dst), planes, unit)
	}
	want := -1
	for _, p := range dst {
		if p != nil && (want < 0 || len(p) < want) {
			want = len(p)
		}
	}
	if want < 0 {
		return 0, errors.New("sampleio: every plane is nil, so there is nothing to read into")
	}
	if planes == 1 {
		// One channel is already contiguous, so it decodes straight into the
		// caller's slice.
		return readOne(r, dst[0])
	}

	total := 0
	for total < want {
		frames, err := r.ready(want - total)
		if err != nil {
			if total > 0 {
				break
			}
			return 0, err
		}
		// Decode the whole interleaved block, then copy out the planes that
		// were asked for.
		w := scratch[T](&r.work, frames*planes)
		pcm.Decode(w, r.buf[r.off:], r.info.Format)
		noteOverrange(r, w)
		for c, p := range dst {
			if p == nil {
				continue
			}
			p = p[total : total+frames]
			src := w[c:]
			for i := range p {
				p[i] = src[i*planes]
			}
		}
		r.advance(frames)
		total += frames
	}
	return total, nil
}

// readOne fills dst from a stream of one channel or one I/Q pair.
func readOne[T dsp.Sample](r *Reader, dst []T) (int, error) {
	total := 0
	for total < len(dst) {
		frames, err := r.ready(len(dst) - total)
		if err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, err
		}
		// The source slice runs to the end of buf; pcm.Decode stops at the
		// shorter of its two slices.
		d := dst[total : total+frames]
		pcm.Decode(d, r.buf[r.off:], r.info.Format)
		noteOverrange(r, d)
		r.advance(frames)
		total += frames
	}
	return total, nil
}

// planes returns how many slices a planar read takes, and what they hold.
func (r *Reader) planes() (int, string) {
	if r.iq {
		return r.elems / 2, "I/Q pairs"
	}
	return r.elems, "channels"
}

// checkKind returns an error when a real read is made of an I/Q stream, or a
// complex read of a real one.
func (r *Reader) checkKind(complexT bool) error {
	switch {
	case complexT == r.iq:
		return nil
	case r.iq:
		return errors.New("sampleio: this stream is I/Q; read it into complex64 or complex128")
	default:
		return errors.New("sampleio: this stream is real; read it into float32 or float64")
	}
}

func isComplex[T dsp.Sample]() bool {
	var z T
	switch any(z).(type) {
	case complex64, complex128:
		return true
	}
	return false
}

// scratch returns a []E of length n kept in *slot, allocating a new one when
// the slot's slice is too short or of another type.
func scratch[E any](slot *any, n int) []E {
	s, _ := (*slot).([]E)
	if cap(s) < n {
		s = make([]E, n)
		*slot = s
	}
	return s[:n]
}

// ready reads from the source until at least one whole frame is buffered and
// returns how many are, up to want. It returns io.EOF at the end of the stream.
func (r *Reader) ready(want int) (int, error) {
	for {
		frames := (r.n - r.off) / r.stride
		if r.total >= 0 {
			frames = int(min(int64(frames), r.total-r.pos))
		}
		if frames > 0 {
			return min(frames, want), nil
		}
		if r.eof || (r.total >= 0 && r.pos >= r.total) {
			return 0, io.EOF
		}
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
}

// fill moves the undecoded bytes to the front of buf and reads more after
// them.
func (r *Reader) fill() error {
	if r.off > 0 {
		r.n = copy(r.buf, r.buf[r.off:r.n])
		r.off = 0
	}
	// An io.Reader may return (0, nil), and a source that does so forever
	// would hang this loop. Give up after as many empty reads as io.ReadFull
	// allows.
	const maxEmptyReads = 100
	empty := 0
	for r.n < r.stride {
		m, err := r.src.Read(r.buf[r.n:])
		r.n += m
		if err != nil {
			if errors.Is(err, io.EOF) {
				r.eof = true
				// Part of a frame at the end is a truncated file. It is
				// dropped.
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
	// Try once to fill the rest of the buffer, without waiting for it.
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

// advance marks frames buffered frames as consumed.
func (r *Reader) advance(frames int) {
	r.off += frames * r.stride
	r.pos += int64(frames)
}
