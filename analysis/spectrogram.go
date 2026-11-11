package analysis

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"

	"samuelks.com/dsp"
)

// STFTOptions describes how a signal is cut up and transformed for a
// spectrogram: the frame, the hop, the window, and how far the result is folded
// down. It decides what the Spectrogram holds; SpectrogramOptions decides how
// that is drawn.
type STFTOptions struct {
	// Size is the transform length, rounded up to a power of two. Zero selects
	// 2048, which at 48 kHz trades about 23 Hz of frequency resolution against
	// 43 ms of time resolution.
	//
	// That default is tuned for a filter sweep and does not suit a recording:
	// what a spectrogram resolves is the frame's length in time, and 2048 points
	// at 11025 Hz is a 186 ms frame, long enough to average 300-baud keying
	// away. Size frames for a capture with FrameSize.
	Size int

	// Hop between frames. Zero selects Size/4.
	Hop int

	// Window is applied to each frame. Nil selects a Kaiser at the shape this
	// package uses for analysis. A spectrogram's frames are never coherent with
	// what is in them, so the window is what sets the noise floor.
	Window func([]float64)

	// FloorDB clips each bin from below. Zero selects -180.
	FloorDB float64

	// Columns bounds the result: frames are folded into at most this many
	// columns, taking the maximum. Zero keeps every frame.
	//
	// It is what lets a capture longer than memory be looked at, since the
	// frames are the part that grows.
	Columns int

	// Complex analyzes an I/Q signal, giving a two-sided spectrogram whose bins
	// run from -rate/2 up through zero. A builder with this set takes
	// WriteComplex and refuses Write.
	Complex bool

	// Center is the frequency an I/Q capture was tuned to. It shifts nothing and
	// is only used to label the plot, whose axis stays in baseband offsets.
	Center float64
}

// FrameSize returns a transform length whose frame covers about seconds of
// signal at rate, rounded to a power of two and clamped to a usable range.
//
// It is how a caller sizes frames for a recording rather than for a filter.
// 25 ms is a reasonable default for finding structure.
func FrameSize(rate, seconds float64) int {
	if rate <= 0 || seconds <= 0 {
		return 2048
	}
	n := rate * seconds
	// The nearest power of two, not the next one up: rounding 1200 up to 2048
	// would double the frame this asked for.
	size := 1
	for float64(size)*2 <= n {
		size <<= 1
	}
	if n/float64(size) > float64(size*2)/n {
		size <<= 1
	}
	return min(max(size, 256), 1<<16)
}

// Spectrogram is a magnitude spectrum per time step, in dBFS.
type Spectrogram struct {
	Frames [][]float64 // Frames[t][bin]

	// Times is the start of each column in seconds. Where several frames were
	// folded into one column it is the first of them.
	Times []float64

	// Freqs is the frequency of each bin in Hz. A two-sided spectrogram's runs
	// from -Rate/2 up through zero, in baseband offsets from Center.
	Freqs []float64

	Rate  float64
	Floor float64
	Peak  float64

	// Center is the frequency an I/Q capture was tuned to, for labeling only.
	Center float64

	// NoiseBW is the equivalent noise bandwidth of one bin in Hz, and SidelobeDB
	// the analysis window's own peak sidelobe, or zero for a window this package
	// did not choose and cannot vouch for. Together they are the floor of the
	// measurement: a spectrogram drawn 180 dB deep over a window that leaks at
	// 90 is showing its window below that.
	NoiseBW    float64
	SidelobeDB float64
}

// TwoSided reports whether the spectrogram covers negative frequencies, which is
// what a complex signal's does. It is derived from Freqs rather than stored, so
// it cannot disagree with them.
func (sg *Spectrogram) TwoSided() bool {
	return len(sg.Freqs) > 0 && sg.Freqs[0] < 0
}

// Reset clears the accumulated frames, so the builder starts a new signal.
func (b *SpectrogramBuilder) Reset() {
	// The options were validated when this builder was made.
	n, err := NewSpectrogramBuilder(b.rate, b.opt)
	if err != nil {
		return
	}
	*b = *n
}

// SpectrogramOf cuts x into overlapping frames and transforms each.
//
// It is the whole-signal form of SpectrogramBuilder and produces exactly what
// feeding the same samples to a builder in any number of pieces produces.
func SpectrogramOf(x []float64, rate float64, opt STFTOptions) (*Spectrogram, error) {
	b, err := NewSpectrogramBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	if err := b.Write(x); err != nil {
		return nil, err
	}
	return b.Close()
}

// SpectrogramOfComplex is SpectrogramOf for an I/Q signal, giving a two-sided
// spectrogram.
func SpectrogramOfComplex(x []complex128, rate float64, opt STFTOptions) (*Spectrogram, error) {
	opt.Complex = true
	b, err := NewSpectrogramBuilder(rate, opt)
	if err != nil {
		return nil, err
	}
	if err := b.WriteComplex(x); err != nil {
		return nil, err
	}
	return b.Close()
}

// SpectrogramBuilder computes a spectrogram from signal fed a piece at a time.
// Each frame is transformed on arrival and then reduced into the columns, so
// nothing proportional to the input length is ever held and a capture larger
// than memory can be spectrogrammed a block at a time. It is not safe for
// concurrent use.
type SpectrogramBuilder struct {
	rate    float64
	opt     STFTOptions
	size    int
	hop     int
	floor   float64
	center  float64
	twoSide bool

	w        []float64
	gain     float64
	gain2    float64
	sidelobe float64

	fft  *dsp.FFT[complex128]
	spec []complex128
	rbuf []float64    // windowed real frame
	cbuf []complex128 // windowed complex frame

	ring  []float64    // carried-over real samples
	cring []complex128 // carried-over complex samples
	have  int          // samples in the ring
	drop  int64        // samples still to discard, when hop exceeds size

	start int64 // index of the next frame's first sample
	total int64 // samples written

	freqs []float64
	cols  columns
	peak  float64
	err   error

	// The power spectrum of every frame, accumulated alongside the decibels the
	// columns keep. It is summed here because the columns hold decibels, which
	// must never be averaged; see psd.go.
	nframes int
	powSum  []float64
	powHi   []float64
	powLo   []float64
}

// NewSpectrogramBuilder starts a spectrogram.
func NewSpectrogramBuilder(rate float64, opt STFTOptions) (*SpectrogramBuilder, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("analysis: a sample rate of %g", rate)
	}
	if opt.Size == 1 {
		// roundPow2 leaves 1 alone, and a one-point transform has no frequency
		// axis, which every consumer of Freqs indexes past. Zero still means the
		// default.
		return nil, fmt.Errorf("analysis: a frame size of %d; two is the smallest transform with a frequency axis", opt.Size)
	}
	size := roundPow2(opt.Size, 2048)
	hop := opt.Hop
	if hop <= 0 {
		hop = size / 4
	}
	if hop <= 0 {
		hop = 1
	}
	floor := floorOr(opt.FloorDB, -180)

	b := &SpectrogramBuilder{
		rate: rate, opt: opt, size: size, hop: hop, floor: floor,
		center: opt.Center, twoSide: opt.Complex,
		peak: math.Inf(-1),
	}

	b.w = make([]float64, size)
	if opt.Window != nil {
		opt.Window(b.w)
	} else {
		dsp.KaiserWindow(b.w, analysisBeta)
		b.sidelobe = analysisSidelobeDB
	}
	b.gain, b.gain2 = dsp.WindowGain(b.w)

	f, err := dsp.NewFFT[complex128](size)
	if err != nil {
		return nil, err
	}
	b.fft = f
	b.spec = make([]complex128, size)

	// A two-sided spectrogram keeps every bin; a one-sided one keeps the half a
	// real signal's conjugate symmetry does not repeat.
	bins := size/2 + 1
	if opt.Complex {
		bins = size
		b.cring = make([]complex128, size)
		b.cbuf = make([]complex128, size)
	} else {
		b.ring = make([]float64, size)
		b.rbuf = make([]float64, size)
	}
	b.freqs = make([]float64, bins)
	for i := range b.freqs {
		k := i
		if opt.Complex {
			// After the shift, bin i carries (i - size/2)*rate/size.
			k = i - size/2
		}
		b.freqs[i] = float64(k) * rate / float64(size)
	}

	b.powSum = make([]float64, bins)
	b.powHi = make([]float64, bins)
	b.powLo = make([]float64, bins)
	for i := range b.powLo {
		b.powHi[i] = math.Inf(-1)
		b.powLo[i] = math.Inf(1)
	}

	b.cols = newColumns(opt.Columns)
	return b, nil
}

// Write adds real samples.
func (b *SpectrogramBuilder) Write(x []float64) error {
	if b.err != nil {
		return b.err
	}
	if b.twoSide {
		return errors.New("analysis: this spectrogram is two-sided; use WriteComplex")
	}
	b.total += int64(len(x))
	for len(x) > 0 {
		if b.drop > 0 {
			n := min(b.drop, int64(len(x)))
			x = x[n:]
			b.drop -= n
			continue
		}
		n := copy(b.ring[b.have:b.size], x)
		b.have += n
		x = x[n:]
		if b.have < b.size {
			return nil
		}
		b.emitReal()
		b.advance()
	}
	return nil
}

// WriteComplex adds I/Q samples.
func (b *SpectrogramBuilder) WriteComplex(x []complex128) error {
	if b.err != nil {
		return b.err
	}
	if !b.twoSide {
		return errors.New("analysis: this spectrogram is one-sided; use Write")
	}
	b.total += int64(len(x))
	for len(x) > 0 {
		if b.drop > 0 {
			n := min(b.drop, int64(len(x)))
			x = x[n:]
			b.drop -= n
			continue
		}
		n := copy(b.cring[b.have:b.size], x)
		b.have += n
		x = x[n:]
		if b.have < b.size {
			return nil
		}
		b.emitComplex()
		b.advance()
	}
	return nil
}

// advance slides the ring on by one hop, which for a hop wider than the frame
// means dropping samples that no frame covers.
func (b *SpectrogramBuilder) advance() {
	b.start += int64(b.hop)
	if b.hop < b.have {
		if b.cring != nil {
			copy(b.cring, b.cring[b.hop:b.have])
		} else {
			copy(b.ring, b.ring[b.hop:b.have])
		}
		b.have -= b.hop
		return
	}
	b.drop = int64(b.hop - b.have)
	b.have = 0
}

// emitReal transforms the frame in the ring and reduces it into the columns.
//
// The arithmetic is the contract: window, transform, magnitude scaled by the
// window's gain and by the two that the mirror bin holds, DC and Nyquist halved
// because they have no mirror, then decibels floored from below. Anything that
// reorders it changes every number this package reports.
func (b *SpectrogramBuilder) emitReal() {
	for i := range b.size {
		b.rbuf[i] = b.ring[i] * b.w[i]
	}
	b.fft.ForwardReal(b.spec, b.rbuf)

	row := make([]float64, len(b.freqs))
	for i := range row {
		m := 2 * cmplx.Abs(b.spec[i]) / b.gain
		if i == 0 || i == b.size/2 {
			m /= 2
		}
		b.power(i, m)
		row[i] = b.dB(m)
	}
	b.nframes++
	b.cols.add(row, float64(b.start)/b.rate)
}

// emitComplex is emitReal for an I/Q frame.
//
// Every bin is kept, since a complex spectrum is not symmetric: its negative
// frequencies are signal, not a mirror. The factor of two goes too, as it made
// up for the energy a real spectrum's mirror bin held; keeping it would report
// every I/Q signal 6.02 dB hot while the picture still looked correct.
func (b *SpectrogramBuilder) emitComplex() {
	for i := range b.size {
		b.cbuf[i] = b.cring[i] * complex(b.w[i], 0)
	}
	b.fft.Forward(b.spec, b.cbuf)
	dsp.FFTShift(b.spec)

	row := make([]float64, len(b.freqs))
	for i := range row {
		m := cmplx.Abs(b.spec[i]) / b.gain
		b.power(i, m)
		row[i] = b.dB(m)
	}
	b.nframes++
	b.cols.add(row, float64(b.start)/b.rate)
}

// power accumulates one bin's linear power, which is what an averaged spectrum
// has to be summed from. See PSD for why the decibels this frame is about to be
// turned into are the wrong thing to average.
func (b *SpectrogramBuilder) power(i int, m float64) {
	p := m * m
	b.powSum[i] += p
	b.powHi[i] = math.Max(b.powHi[i], p)
	b.powLo[i] = math.Min(b.powLo[i], p)
}

// dB converts a magnitude, applies the floor and tracks the peak.
func (b *SpectrogramBuilder) dB(m float64) float64 {
	db := dbAmplitude(m)
	if db < b.floor || math.IsNaN(db) {
		db = b.floor
	}
	b.peak = math.Max(b.peak, db)
	return db
}

// Close finishes the spectrogram. The builder must not be written to again.
func (b *SpectrogramBuilder) Close() (*Spectrogram, error) {
	if b.err != nil {
		return nil, b.err
	}
	frames, times := b.cols.result()
	if len(frames) == 0 {
		b.err = fmt.Errorf("analysis: %d samples is fewer than one %d point frame", b.total, b.size)
		return nil, b.err
	}
	peak := b.peak
	if math.IsInf(peak, -1) {
		peak = 0
	}
	sg := &Spectrogram{
		Frames:     frames,
		Times:      times,
		Freqs:      b.freqs,
		Rate:       b.rate,
		Floor:      b.floor,
		Peak:       peak,
		Center:     b.center,
		SidelobeDB: b.sidelobe,
	}
	if b.gain > 0 {
		sg.NoiseBW = b.rate * b.gain2 / (b.gain * b.gain)
	}
	return sg, nil
}

// columns folds frames into a bounded number of columns, taking the maximum.
//
// When the buffer fills, adjacent pairs merge and the frames per column
// doubles, so a stream of unknown length reduces in a single pass; the count
// lands between cap/2 and cap.
//
// The maximum rather than the mean, as Image downscales: a spur is one frame
// and one bin wide, and averaging it against silent neighbors shows a clean
// band that is not there.
//
// Max folding is associative and idempotent, so folding frames into columns and
// then columns into pixels matches the straight fold when the column boundaries
// line up with the pixel grid -- when the count is a whole multiple of the pane
// width. Misaligned, a feature can move by up to a column, as under any
// resampling, but no peak is lost.
type columns struct {
	cap       int // 0 for unbounded
	framesPer int
	cols      [][]float64
	times     []float64
	total     int64
}

func newColumns(maxCols int) columns {
	if maxCols > 0 {
		// Forced even so a merge always pairs cleanly and no column is left over.
		maxCols += maxCols % 2
		if maxCols < 2 {
			maxCols = 2
		}
	}
	return columns{cap: maxCols, framesPer: 1}
}

func (c *columns) add(row []float64, t float64) {
	if c.cap <= 0 {
		c.cols = append(c.cols, row)
		c.times = append(c.times, t)
		c.total++
		return
	}
	idx := int(c.total / int64(c.framesPer))
	if idx == len(c.cols) {
		// A new column. Merging when the buffer is full leaves the index still
		// pointing one past the end, so this row starts a column either way.
		if len(c.cols) == c.cap {
			c.merge()
		}
		c.cols = append(c.cols, row)
		c.times = append(c.times, t)
	} else {
		into := c.cols[idx]
		for i := range into {
			into[i] = math.Max(into[i], row[i])
		}
	}
	c.total++
}

// merge folds adjacent pairs of columns together, halving the count.
func (c *columns) merge() {
	half := len(c.cols) / 2
	for i := range half {
		a, b := c.cols[2*i], c.cols[2*i+1]
		for j := range a {
			a[j] = math.Max(a[j], b[j])
		}
		c.cols[i] = a
		c.times[i] = c.times[2*i]
	}
	c.cols = c.cols[:half]
	c.times = c.times[:half]
	c.framesPer *= 2
}

func (c *columns) result() ([][]float64, []float64) { return c.cols, c.times }
