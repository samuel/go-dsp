package dtmf

import (
	"github.com/samuel/go-sdr/sdr"
)

var (
	Keypad = []rune{
		'1', '2', '3', 'A',
		'4', '5', '6', 'B',
		'7', '8', '9', 'C',
		'*', '0', '#', 'D',
	}
	StdLowFreq  = []int{697, 770, 852, 941}
	StdHighFreq = []int{1209, 1336, 1477, 1633}
)

type DTMF struct {
	lowFreq  *sdr.Goertzel
	highFreq *sdr.Goertzel
	nHigh    int
}

func New(lowFreq, highFreq []int, sampleRate, blockSize int) *DTMF {
	return &DTMF{
		lowFreq:  sdr.NewGoertzel(lowFreq, sampleRate, blockSize),
		highFreq: sdr.NewGoertzel(highFreq, sampleRate, blockSize),
		nHigh:    len(highFreq),
	}
}

func NewStandard(sampleRate, blockSize int) *DTMF {
	return New(StdLowFreq, StdHighFreq, sampleRate, blockSize)
}

// Return key number (lowFreqIndex * numHighFreq + highFreqIndex) and minimum threshold
func (d *DTMF) Feed(samples []float32) (int, float32) {
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
