package sampleio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"samuelks.com/dsp"
	"samuelks.com/dsp/internal/view"
	"samuelks.com/dsp/pcm"
)

// Channel selects which channel of a stream to read.
type Channel int

// MixChannels selects the mean of every channel rather than one of them.
const MixChannels Channel = -1

// blockBytes is the raw read size. Large enough that a read is not dominated by
// syscall overhead, small enough that a Reader costs nothing to hold.
const blockBytes = 1 << 16

// Info describes what a Reader found. Its Layout is the stream as read: the
// header's where there is one, with any Rate override applied, and Container
// WAV or Raw.
type Info struct {
	Layout

	// Frames is how many samples the Reader will produce, after Skip and Limit, or
	// -1 when that cannot be known without reading the whole stream.
	//
	// It is a length, not a guess: a header's claim is believed only once checked
	// against the size on disk. A pipe therefore reports -1 even when its header
	// states a size, and so does a file whose header overstates its data, with a
	// Note.
	Frames int64

	Bits      int // container bits per sample; 0 for a float format
	ValidBits int // significant bits, when the header says fewer than Bits

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
	// Layout describes a headerless stream; see its fields for how each one
	// combines with a WAV header.
	Layout

	// Channel selects which one to read. MixChannels takes the mean of all of
	// them. With IQ set it selects an I/Q pair rather than a single channel.
	Channel Channel

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

	gath []byte // one channel's bytes, gathered contiguously

	// work is the decoded scratch a mixdown or planar read needs, a []T at the
	// width of the last read that used it. views is a planar read's [][]T of
	// plane headers: dsp.Deinterleave writes at the front of the planes it is
	// handed, so a block landing mid-plane re-points these instead of passing
	// an offset.
	work  any
	views any

	// overrange is set once a float sample outside full scale has been decoded;
	// see noteOverrange.
	overrange bool

	left int64 // frames still allowed by Limit; -1 for no limit
	eof  bool
}

// Open reads the file at path. "-" or "" reads standard input, whose length is
// not known ahead of time.
//
// A file is read as a WAV when it starts with one, unless Options.Container
// says otherwise.
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

	if opt.Container < DetectContainer || opt.Container > Raw {
		return nil, fmt.Errorf("sampleio: %v is not a container", opt.Container)
	}

	r := &Reader{src: src, closer: closer, left: -1}
	r.info = Info{Container: Raw, Frames: -1}

	// A header, unless the caller said not to look for one.
	var dataBytes int64 = -1
	if opt.Container != Raw {
		h, err := sniffWAV(&r.src)
		if err != nil {
			return nil, err
		}
		if h == nil && opt.Container == WAV {
			return nil, errors.New("sampleio: the stream does not start with a WAV header")
		}
		if h != nil {
			r.info.Container = WAV
			r.info.Format = h.format
			r.info.Rate = h.rate
			r.info.Channels = h.channels
			r.info.Bits = h.bits
			r.info.ValidBits = h.validBits
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

	if r.info.Container == Raw {
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
		r.info.Bits = opt.Format.Bits()
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
	r.info.IQ = opt.IQ
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
	case r.info.Container == Raw && size >= 0:
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

// ReadSamples fills dst with samples of the selected channel, or I/Q pair, and
// returns how many it wrote. It returns io.EOF only when it wrote nothing.
//
// The samples are decoded straight into dst at its width. T is real for a real
// stream and complex for an I/Q one; the other is an error, since handing back
// only the real part of an I/Q stream would be worse than saying so.
//
// It is not io.Reader's Read: the unit is a sample of one channel rather than
// a byte, so the count means something different.
func (r *Reader) ReadSamples[T dsp.Sample](dst []T) (int, error) {
	if err := r.checkKind(isComplex[T]()); err != nil {
		return 0, err
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
		decodeSelected(r, dst[total:total+frames], frames)
		r.advance(frames)
		total += frames
	}
	return total, nil
}

// ReadPlanar fills one slice per channel with that channel's samples and
// returns how many frames it wrote to each. It returns io.EOF only when it
// wrote nothing.
//
// dst holds one plane per channel of the file in file order, or one per I/Q
// pair when T is complex, and must hold exactly as many as the stream has.
// Options.Channel is not consulted: this is the whole frame rather than a
// selection, and a Reader opened for a mixdown is an error rather than a
// silent choice between the two.
//
// It decodes the interleaved run once and splits it in one pass, so it is
// several times faster than a Reader per channel, which reads the file once
// per channel.
//
// Planes may differ in length; the shortest is what gets read, and the frames
// that do not fit are not consumed either. The planes must not overlap.
func (r *Reader) ReadPlanar[T dsp.Sample](dst [][]T) (int, error) {
	if err := r.checkKind(isComplex[T]()); err != nil {
		return 0, err
	}
	if r.pick == MixChannels {
		return 0, errors.New("sampleio: this Reader mixes its channels down, and a mixdown has no planar form")
	}
	planes, unit := r.elems, "channels"
	if r.iq {
		planes, unit = r.elems/2, "I/Q pairs"
	}
	if len(dst) != planes {
		return 0, fmt.Errorf("sampleio: %d planes for a stream of %d %s", len(dst), planes, unit)
	}
	if planes == 1 {
		// One channel is already contiguous, so this is a decode straight into
		// the caller's slice, with no split.
		return r.ReadSamples(dst[0])
	}

	want := len(dst[0])
	for _, p := range dst[1:] {
		want = min(want, len(p))
	}
	heads := scratch[[]T](&r.views, planes)
	total := 0
	for total < want {
		frames, err := r.ready(want - total)
		if err != nil {
			if total > 0 {
				break
			}
			return 0, err
		}
		for c, p := range dst {
			heads[c] = p[total : total+frames]
		}
		w := scratch[T](&r.work, frames*planes)
		pcm.Decode(w, r.buf[r.off:], r.info.Format)
		noteOverrange(r, w)
		dsp.Deinterleave(heads, w)
		r.advance(frames)
		total += frames
	}
	// The arrays behind these headers are the caller's; leaving them pointed at
	// the planes would keep them alive as long as the Reader is held.
	clear(heads)
	return total, nil
}

// checkKind reports whether a read at a real or complex width fits the stream.
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

// scratch returns n elements of the []E held in *slot, replacing it when it is
// short or holds another type. Storing a slice in an interface allocates, so
// it is done only on growth.
func scratch[E any](slot *any, n int) []E {
	s, _ := (*slot).([]E)
	if cap(s) < n {
		s = make([]E, n)
		*slot = s
	}
	return s[:n]
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

// decodeSelected writes frames samples of the selected channel or I/Q pair
// into dst.
func decodeSelected[T dsp.Sample](r *Reader, dst []T, frames int) {
	src := r.buf[r.off:]
	f := r.info.Format
	first, count := int(r.pick), 1
	if r.iq {
		first, count = 2*first, 2
	}

	switch {
	case r.pick == MixChannels:
		// Only a real stream reaches here: selectChannel refuses to mix I/Q.
		w := scratch[float64](&r.work, frames*r.elems)
		pcm.Decode(w, src, f)
		noteOverrange(r, w)
		switch d := any(dst).(type) {
		case []float64:
			mixdown(d, w, r.elems)
		case []float32:
			mixdown(d, w, r.elems)
		}
		return

	case r.elems == count:
		// The common case: one channel or one pair, so the samples are already
		// contiguous and the decode is one call over the whole run. src runs to
		// the end of buf, and pcm.Decode stops at the shorter of its two
		// slices.
		pcm.Decode(dst, src, f)

	default:
		pcm.Decode(dst, r.gather(src, frames, first, count), f)
	}
	noteOverrange(r, dst)
}

// mixdown writes the mean of each frame of the interleaved run w to dst. It
// sums in float64 and narrows once, so a float32 mixdown is the float64 one
// narrowed.
func mixdown[F dsp.Float](dst []F, w []float64, channels int) {
	inv := 1 / float64(channels)
	for i := range dst {
		var sum float64
		for _, v := range w[i*channels : (i+1)*channels] {
			sum += v
		}
		dst[i] = F(sum * inv)
	}
}

// gather copies count elements per frame, starting at element first, into one
// contiguous run.
//
// It is a byte shuffle rather than a strided decode so the decode kernels only
// ever see contiguous input; the mono case and a planar read skip it. It
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
