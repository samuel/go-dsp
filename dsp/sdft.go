package dsp

import (
	"math"
	"math/cmplx"
)

// TODO: damping

// SDFT is a sliding DFT.
type SDFT struct {
	i int
	w []complex128
	s []complex128
	x []complex128
	e []complex128
}

func NewSDFT(k, n int, window []float64) *SDFT {
	var win []complex128
	if len(window) == 0 {
		win = []complex128{complex(1, 0)}
	} else {
		win = make([]complex128, len(window))
		for i, w := range window {
			win[i] = complex(w, 0)
		}
	}
	s := &SDFT{
		w: win,
		x: make([]complex128, n),
		e: make([]complex128, len(win)),
		s: make([]complex128, len(win)),
	}
	for i := 0; i < len(win); i++ {
		j := k - len(win)/2 + i
		if j < 0 {
			j += n
		} else if j >= n {
			j -= n
		}
		s.e[i] = cmplx.Exp(complex(0, 2*math.Pi*float64(j)/float64(n)))
	}
	return s
}

func (sd *SDFT) Filter(x complex128) complex128 {
	i := (sd.i + 1) % len(sd.x)
	x0 := sd.x[i]
	sd.x[i] = x
	sd.i = i
	xd := x - x0
	var sum complex128
	for i, w := range sd.w {
		s := (xd + sd.s[i]) * sd.e[i]
		sd.s[i] = s
		sum += w * s
	}
	return sum
}

// SDFT32 is a 32-bit float version of a sliding DFT.
type SDFT32 struct {
	i int
	w []complex64
	s []complex64
	x []complex64
	e []complex64
}

func NewSDFT32(k, n int, window []float32) *SDFT32 {
	var win []complex64
	if len(window) == 0 {
		win = []complex64{complex(1, 0)}
	} else {
		win = make([]complex64, len(window))
		for i, w := range window {
			win[i] = complex(w, 0)
		}
	}
	s := &SDFT32{
		w: win,
		x: make([]complex64, n),
		e: make([]complex64, len(win)),
		s: make([]complex64, len(win)),
	}
	for i := 0; i < len(win); i++ {
		j := k - len(win)/2 + i
		if j < 0 {
			j += n
		} else if j >= n {
			j -= n
		}
		s.e[i] = complex64(cmplx.Exp(complex(0, 2*math.Pi*float64(j)/float64(n))))
	}
	return s
}

func (sd *SDFT32) Filter(x complex64) complex64 {
	i := (sd.i + 1) % len(sd.x)
	x0 := sd.x[i]
	sd.x[i] = x
	sd.i = i
	xd := x - x0
	var sum complex64
	for i, w := range sd.w {
		s := (xd + sd.s[i]) * sd.e[i]
		sd.s[i] = s
		sum += w * s
	}
	return sum
}
