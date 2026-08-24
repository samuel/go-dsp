// Package dtmf decodes dual-tone multi-frequency signaling (DTMF), the
// two-tone signals a telephone keypad sends for each key: one tone from
// a low group of four frequencies and one from a high group of four,
// sounded together, giving sixteen keys.
//
// A Decoder verifies each block of samples before reporting a digit: both
// tones must carry enough of the block's energy, each must stand clearly above
// the other tones in its group, and the two groups must balance within
// tolerance. Anything else -- silence, noise, speech, a partial digit --
// reports NoKey rather than guessing the nearest key.
package dtmf

import (
	"errors"
	"slices"
	"strconv"

	"github.com/samuel/go-dsp/dsp"
)

// Unexported so that an importer cannot rewrite them for the whole process;
// Key and StandardFreqs hand out copies.
var (
	keypad = [16]rune{
		'1', '2', '3', 'A',
		'4', '5', '6', 'B',
		'7', '8', '9', 'C',
		'*', '0', '#', 'D',
	}
	stdLowFreq  = [4]float64{697, 770, 852, 941}
	stdHighFreq = [4]float64{1209, 1336, 1477, 1633}
)

// NoKey is what Feed returns for a block that does not hold a valid digit.
const NoKey = -1

// Key returns the character for a key number from a decoder built on the
// standard frequencies, or zero for NoKey and for anything else out of range.
func Key(n int) rune {
	if n < 0 || n >= len(keypad) {
		return 0
	}
	return keypad[n]
}

// StandardFreqs returns the two standard groups of tone frequencies in Hz, low
// group first, as fresh slices.
func StandardFreqs() (low, high []float64) {
	return slices.Clone(stdLowFreq[:]), slices.Clone(stdHighFreq[:])
}

// The validity tests a block has to pass. All three are ratios, so they hold at
// any signal level.
const (
	// minTonePower is how much of the block's energy each of the two tones has
	// to carry, as a fraction of what a single windowed sinusoid at a bin
	// center would give. It rejects silence and low-level noise. See toneGain
	// for what makes that fraction mean the same thing under any window.
	minTonePower = 0.05

	// relativePeak is how far the runner-up tone in a group has to sit below
	// the winner, as a power ratio -- 8 dB. Broadband noise and speech spread
	// power across the group and fail it.
	relativePeak = 6.3

	// maxHighOverLow and maxLowOverHigh bound the twist, the imbalance between
	// the two groups: 8 dB with the high tone stronger, 4 dB with the low one
	// stronger. The tolerance is asymmetric because handsets emit the high
	// group a little hot and the network attenuates it more.
	maxHighOverLow = 6.3
	maxLowOverHigh = 2.5
)

// Decoder detects one digit per block of samples. It is not safe for concurrent
// use, and it holds scratch buffers sized for its block, so one Decoder follows
// one stream.
type Decoder struct {
	lowFreq   *dsp.Goertzel[float32]
	highFreq  *dsp.Goertzel[float32]
	nHigh     int
	blockSize int
	// toneGain is the bin power a sinusoid at a bin center produces divided by
	// the energy of the same block after windowing, which is
	// sum(w)^2 / (2*sum(w^2)). Scaling the floor by it is what makes
	// minTonePower a fraction of a real tone rather than a fraction of an
	// arbitrary constant: a rectangular window and a Hamming differ by 1.4x
	// here, so a caller-supplied window would otherwise move the threshold
	// without saying so.
	toneGain float64
	w        []float32
	// windowed holds the windowed block. Windowing the caller's slice in place
	// would corrupt any sliding buffer it feeds from, re-windowing the samples
	// each block retains.
	windowed []float32
}

// New returns a Decoder for the given tone groups, in Hz. A key number is
// lowFreqIndex*len(highFreq) + highFreqIndex, so for the standard groups it
// indexes the keypad Key returns from. windowFunc fills a window of blockSize
// points; nil means a Hamming window.
func New(lowFreq, highFreq []float64, sampleRate float64, blockSize int, windowFunc func([]float32)) (*Decoder, error) {
	if len(lowFreq) == 0 || len(highFreq) == 0 {
		return nil, errors.New("dtmf: need at least one frequency in each group")
	}
	// Before the allocation below, which panics on a negative length rather
	// than reporting it. NewGoertzel checks both too, but only after that.
	if blockSize <= 0 {
		return nil, errors.New("dtmf: block size must be positive")
	}
	if sampleRate <= 0 {
		return nil, errors.New("dtmf: sample rate must be positive")
	}
	w := make([]float32, blockSize)
	if windowFunc != nil {
		windowFunc(w)
	} else {
		dsp.HammingWindow(w)
	}
	var sum, sumSq float64
	for _, v := range w {
		sum += float64(v)
		sumSq += float64(v) * float64(v)
	}
	if sumSq <= 0 {
		return nil, errors.New("dtmf: the window is all zeros")
	}
	lowG, err := dsp.NewGoertzel[float32](lowFreq, sampleRate, blockSize)
	if err != nil {
		return nil, err
	}
	highG, err := dsp.NewGoertzel[float32](highFreq, sampleRate, blockSize)
	if err != nil {
		return nil, err
	}
	return &Decoder{
		lowFreq:   lowG,
		highFreq:  highG,
		nHigh:     len(highFreq),
		blockSize: blockSize,
		toneGain:  sum * sum / (2 * sumSq),
		w:         w,
		windowed:  make([]float32, blockSize),
	}, nil
}

// NewStandard returns a Decoder for the standard keypad frequencies.
func NewStandard(sampleRate float64, blockSize int) (*Decoder, error) {
	low, high := StandardFreqs()
	return New(low, high, sampleRate, blockSize, dsp.HammingWindow)
}

// Feed decodes one block of exactly blockSize samples, which it does not
// modify, and returns the key number together with the power of the weaker of
// the two tones. A block that does not hold a valid digit gives NoKey and a
// power of zero.
//
// It panics on a block of the wrong length: a short one would be tapered by
// the wrong part of the window, and a long one silently ignores its tail.
func (d *Decoder) Feed(src []float32) (int, float32) {
	if len(src) != d.blockSize {
		panic("dtmf: Feed needs exactly " + strconv.Itoa(d.blockSize) + " src, got " + strconv.Itoa(len(src)))
	}

	var energy float64
	for i, s := range src {
		v := s * d.w[i]
		d.windowed[i] = v
		energy += float64(v) * float64(v)
	}
	if energy <= 0 {
		return NoKey, 0
	}

	d.lowFreq.Reset()
	d.highFreq.Reset()
	d.lowFreq.Feed(d.windowed)
	d.highFreq.Feed(d.windowed)

	row, rowPower, rowRunnerUp := argmax(d.lowFreq.Power())
	col, colPower, colRunnerUp := argmax(d.highFreq.Power())

	// Scaling the floor by the block's own energy makes the test independent of
	// how loud the signal is, and by toneGain makes it independent of the block
	// length and of the window.
	floor := minTonePower * energy * d.toneGain
	switch {
	case rowPower < floor || colPower < floor:
	case rowRunnerUp*relativePeak > rowPower || colRunnerUp*relativePeak > colPower:
	case colPower > rowPower*maxHighOverLow || rowPower > colPower*maxLowOverHigh:
	default:
		return row*d.nHigh + col, float32(min(rowPower, colPower))
	}
	return NoKey, 0
}

// argmax returns the index of the largest value in val, that value, and the
// largest of the others.
func argmax(val []float64) (idx int, best, runnerUp float64) {
	for i, f := range val {
		switch {
		case f > best:
			idx, best, runnerUp = i, f, best
		case f > runnerUp:
			runnerUp = f
		}
	}
	return idx, best, runnerUp
}
