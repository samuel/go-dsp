package dtmf

import (
	"github.com/samuel/go-dsp/dsp"
)

var (
	Keypad = []rune{
		'1', '2', '3', 'A',
		'4', '5', '6', 'B',
		'7', '8', '9', 'C',
		'*', '0', '#', 'D',
	}
	StdLowFreq  = []uint64{697, 770, 852, 941}
	StdHighFreq = []uint64{1209, 1336, 1477, 1633}
)

type DTMF struct {
	lowFreq   *dsp.Goertzel32
	highFreq  *dsp.Goertzel32
	nHigh     int
	blockSize int
	w         []float32
}

func New(lowFreq, highFreq []uint64, sampleRate, blockSize int, windowFunc func([]float32)) *DTMF {
	w := make([]float32, blockSize)
	if windowFunc != nil {
		windowFunc(w)
	} else {
		dsp.HammingWindowF32(w)
	}
	return &DTMF{
		lowFreq:   dsp.NewGoertzel32(lowFreq, sampleRate, blockSize),
		highFreq:  dsp.NewGoertzel32(highFreq, sampleRate, blockSize),
		nHigh:     len(highFreq),
		blockSize: blockSize,
		w:         w,
	}
}

func NewStandard(sampleRate, blockSize int) *DTMF {
	return New(StdLowFreq, StdHighFreq, sampleRate, blockSize, dsp.HammingWindowF32)
}

// Return key number (lowFreqIndex * numHighFreq + highFreqIndex) and minimum magnitude
func (d *DTMF) Feed(samples []float32) (int, float32) {
	if len(samples) > d.blockSize {
		samples = samples[:d.blockSize]
	}
	for i, s := range samples {
		samples[i] = s * d.w[i]
	}
	d.lowFreq.Reset()
	d.highFreq.Reset()
	d.lowFreq.Feed(samples)
	d.highFreq.Feed(samples)
	row, thresh1 := max(d.lowFreq.Magnitude())
	col, thresh2 := max(d.highFreq.Magnitude())
	if thresh2 < thresh1 {
		thresh1 = thresh2
	}
	return row*d.nHigh + col, thresh1
}

func max(val []float32) (int, float32) {
	lrg := float32(0.0)
	idx := 0
	for i, f := range val {
		if f > lrg {
			lrg = f
			idx = i
		}
	}
	return idx, lrg
}
