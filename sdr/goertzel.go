package sdr

import "math"

type goertzel struct {
	coeff    float32
	cos, sin float32
	q1, q2   float32
}

type Goertzel struct {
	freq []*goertzel
	mag  []float32
	cplx []complex64
}

func NewGoertzel(targetFreqs []int, sampleRate, blockSize int) *Goertzel {
	freq := make([]*goertzel, len(targetFreqs))
	for i, f := range targetFreqs {
		k := int(0.5 + float64(blockSize*f)/float64(sampleRate))
		w := 2.0 * math.Pi * float64(k) / float64(blockSize)

		// norm := float64(f) / float64(sampleRate)
		// w := 2.0 * math.Pi * norm

		sin := float32(math.Sin(w))
		cos := float32(math.Cos(w))
		freq[i] = &goertzel{
			coeff: 2.0 * cos,
			cos:   cos,
			sin:   sin,
		}
	}
	return &Goertzel{
		freq: freq,
		mag:  make([]float32, len(targetFreqs)),
		cplx: make([]complex64, len(targetFreqs)),
	}
}

func (g *Goertzel) Reset() {
	for _, freq := range g.freq {
		freq.q1 = 0.0
		freq.q2 = 0.0
	}
}

func (g *Goertzel) Feed(samples []float32) {
	for _, samp := range samples {
		for _, freq := range g.freq {
			q0 := freq.coeff*freq.q1 - freq.q2 + samp
			freq.q2 = freq.q1
			freq.q1 = q0
		}
	}
}

func (g *Goertzel) Magnitude() []float32 {
	for i, freq := range g.freq {
		g.mag[i] = freq.q1*freq.q1 + freq.q2*freq.q2 - freq.q1*freq.q2*freq.coeff
	}
	return g.mag
}

func (g *Goertzel) Complex() []complex64 {
	for i, freq := range g.freq {
		g.cplx[i] = complex(freq.q1-freq.q2*freq.cos, freq.q2*freq.sin)
	}
	return g.cplx
}
