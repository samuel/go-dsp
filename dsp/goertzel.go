package dsp

import "math"

type Goertzel struct {
	freq []*goertzel
	mag  []float64
	cplx []complex128
}

type Goertzel32 struct {
	freq []*goertzel
	mag  []float32
	cplx []complex64
}

type ComplexGoertzel struct {
	freq []*goertzel
	mag  []float64
	cplx []complex128
}

type goertzel struct {
	coeff    float64
	cos, sin float64
	q1, q2   float64
	q1i, q2i float64
}

func NewGoertzel32(targetFreqs []uint64, sampleRate, blockSize int) *Goertzel32 {
	freq := make([]*goertzel, len(targetFreqs))
	for i, f := range targetFreqs {
		// k is the closest bucket for the frequency
		k := uint64(0.5 + float64(uint64(blockSize)*f)/float64(sampleRate))
		w := 2.0 * math.Pi * float64(k) / float64(blockSize)
		sin := math.Sin(w)
		cos := math.Cos(w)
		freq[i] = &goertzel{
			coeff: 2.0 * cos,
			cos:   cos,
			sin:   sin,
		}
	}
	return &Goertzel32{
		freq: freq,
		mag:  make([]float32, len(targetFreqs)),
		cplx: make([]complex64, len(targetFreqs)),
	}
}

func (g *Goertzel32) Reset() {
	for _, freq := range g.freq {
		freq.q1 = 0.0
		freq.q2 = 0.0
	}
}

func (g *Goertzel32) Feed(samples []float32) {
	for _, samp := range samples {
		for _, freq := range g.freq {
			q0 := freq.coeff*freq.q1 - freq.q2 + float64(samp)
			freq.q2 = freq.q1
			freq.q1 = q0
		}
	}
}

func (g *Goertzel32) Magnitude() []float32 {
	for i, freq := range g.freq {
		g.mag[i] = float32(freq.q1*freq.q1 + freq.q2*freq.q2 - freq.q1*freq.q2*freq.coeff)
	}
	return g.mag
}

func (g *Goertzel32) Complex() []complex64 {
	for i, freq := range g.freq {
		g.cplx[i] = complex(float32(freq.q1*freq.cos-freq.q2), float32(freq.q1*freq.sin))
	}
	return g.cplx
}

func NewGoertzel(targetFreqs []uint64, sampleRate, blockSize int) *Goertzel {
	freq := make([]*goertzel, len(targetFreqs))
	for i, f := range targetFreqs {
		// k is the closest bucket for the frequency
		k := uint64(0.5 + float64(uint64(blockSize)*f)/float64(sampleRate))
		w := 2.0 * math.Pi * float64(k) / float64(blockSize)
		sin := math.Sin(w)
		cos := math.Cos(w)
		freq[i] = &goertzel{
			coeff: 2.0 * cos,
			cos:   cos,
			sin:   sin,
		}
	}
	return &Goertzel{
		freq: freq,
		mag:  make([]float64, len(targetFreqs)),
		cplx: make([]complex128, len(targetFreqs)),
	}
}

func (g *Goertzel) Reset() {
	for _, freq := range g.freq {
		freq.q1 = 0.0
		freq.q2 = 0.0
	}
}

func (g *Goertzel) Feed(samples []float64) {
	for _, samp := range samples {
		for _, freq := range g.freq {
			q0 := freq.coeff*freq.q1 - freq.q2 + samp
			freq.q2 = freq.q1
			freq.q1 = q0
		}
	}
}

func (g *Goertzel) Magnitude() []float64 {
	for i, freq := range g.freq {
		g.mag[i] = freq.q1*freq.q1 + freq.q2*freq.q2 - freq.q1*freq.q2*freq.coeff
	}
	return g.mag
}

func (g *Goertzel) Complex() []complex128 {
	for i, freq := range g.freq {
		g.cplx[i] = complex(freq.q1*freq.cos-freq.q2, freq.q1*freq.sin)
	}
	return g.cplx
}

func NewComplexGoertzel(targetFreqs []uint64, sampleRate, blockSize int) *ComplexGoertzel {
	freq := make([]*goertzel, len(targetFreqs))
	for i, f := range targetFreqs {
		k := uint64(0.5 + float64(uint64(blockSize)*f)/float64(sampleRate))
		w := 2.0 * math.Pi * float64(k) / float64(blockSize)
		sin := math.Sin(w)
		cos := math.Cos(w)
		freq[i] = &goertzel{
			coeff: 2.0 * cos,
			cos:   cos,
			sin:   sin,
		}
	}
	return &ComplexGoertzel{
		freq: freq,
		mag:  make([]float64, len(targetFreqs)),
		cplx: make([]complex128, len(targetFreqs)),
	}
}

func (g *ComplexGoertzel) Reset() {
	for _, freq := range g.freq {
		freq.q1 = 0.0
		freq.q2 = 0.0
		freq.q1i = 0.0
		freq.q2i = 0.0
	}
}

func (g *ComplexGoertzel) Feed(samples []complex128) {
	for _, samp := range samples {
		for _, freq := range g.freq {
			q0 := freq.coeff*freq.q1 - freq.q2 + real(samp)
			freq.q2 = freq.q1
			freq.q1 = q0
			q0 = freq.coeff*freq.q1i - freq.q2i + imag(samp)
			freq.q2i = freq.q1i
			freq.q1i = q0
		}
	}
}

func (g *ComplexGoertzel) Magnitude() []float64 {
	for i, f := range g.freq {
		re := f.q1*f.cos - f.q2 - f.q1i*f.sin
		im := f.q1*f.sin + f.q1i*f.cos - f.q2i
		// q1*cos - q2 - q1i*sin
		g.mag[i] = re*re + im*im
	}
	return g.mag
}

func (g *ComplexGoertzel) Complex() []complex128 {
	for i, f := range g.freq {
		g.cplx[i] = complex(
			f.q1*f.cos-f.q2-f.q1i*f.sin,
			f.q1*f.sin+f.q1i*f.cos-f.q2i,
		)
	}
	return g.cplx
}

// Sliding Goertzel implements a sliding version of the Goertzel filter.
//
// 	x(n)                                               y(n)
// 	──────┬──────(+)──(+)────────────────┬────────(+)─────▶
// 	      ▼       ▲    ▲ ▼               ▼         ▲
// 	    ┌───┐     │    │  ╲            ┌───┐       │
// 	    │z⁻ⁿ│     │    │   ╲           │z⁻ⁿ│       │
// 	    └─┬─┘     │    │    ╲          └─┬─┘       │
// 	      └─▶(x)──┘    │     ╲           │         │
// 	                   │      (x)◀───────●───────▶(x)
// 	                   │       ▲         │         ▲
// 	                   │       │       ┌─▼─┐       │
// 	                   │  2cos(2πk/N)  │z⁻ⁿ│  -e^(-j2πk/N)
// 	                   │               └─┬─┘
// 	                   └──────(x)◀───────┘
// 	                           ▲
// 	                           │
// 	                          -1
// TODO
// type SlidingGoertzel struct {
// }
// func NewSlidingGoertzel(k, n int) *SlidingGoertzel {
// 	return &SlidingGoertzel{}
// }
