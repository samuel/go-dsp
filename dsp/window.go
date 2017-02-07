package dsp

import (
	"math"
	"strconv"
)

var (
	BlackmanFreqCoeff = []float64{0.16 / 4, -1.0 / 4, (1 - 0.16) / 2, -1.0 / 4, 0.16 / 4}
	HammingFreqCoeff  = []float64{(0.53836 - 1) / 2, 0.53836, (0.53836 - 1) / 2}
	HanningFreqCoeff  = []float64{-0.25, 0.5, -0.25}

	BlackmanFreqCoeff32 = []float32{0.16 / 4, -1.0 / 4, (1 - 0.16) / 2, -1.0 / 4, 0.16 / 4}
	HammingFreqCoeff32  = []float32{(0.53836 - 1) / 2, 0.53836, (0.53836 - 1) / 2}
	HanningFreqCoeff32  = []float32{-0.25, 0.5, -0.25}
)

func TriangleWindow(output []float64) {
	for n := range output {
		output[n] = 1 - math.Abs((float64(n)-float64(len(output)-1)/2.0)/(float64(len(output)+1)/2.0))
	}
}

func TriangleWindowF32(output []float32) {
	for n := range output {
		output[n] = float32(1 - math.Abs((float64(n)-float64(len(output)-1)/2.0)/(float64(len(output)+1)/2.0)))
	}
}

func HammingWindow(output []float64) {
	window(output, []float64{0.53836, 1 - 0.53836})
}

func HammingWindowF32(output []float32) {
	windowF32(output, []float64{0.53836, 1 - 0.53836})
}

func HanningWindow(output []float64) {
	for n := range output {
		output[n] = 0.5 * (1 - math.Cos(2*math.Pi*float64(n)/float64(len(output)-1)))
	}
}

func HanningWindowF32(output []float32) {
	for n := range output {
		output[n] = float32(0.5 * (1 - math.Cos(2*math.Pi*float64(n)/float64(len(output)-1))))
	}
}

func BlackmanWindow(output []float64) {
	a := 0.16
	window(output, []float64{(1.0 - a) / 2.0, 1.0 / 2.0, a / 2.0})
}

func BlackmanWindowF32(output []float32) {
	a := 0.16
	windowF32(output, []float64{(1.0 - a) / 2.0, 1.0 / 2.0, a / 2.0})
}

func NuttallWindow(output []float64) {
	window(output, []float64{0.355768, 0.487396, 0.144232, 0.012604})
}

func NuttallWindowF32(output []float32) {
	windowF32(output, []float64{0.355768, 0.487396, 0.144232, 0.012604})
}

func window(output []float64, a []float64) {
	if len(a) < 1 || len(a) > 4 {
		panic("invalid window length " + strconv.Itoa(len(a)))
	}
	nn := float64(len(output) - 1)
	for n := range output {
		fn := float64(n)
		v := a[0]
		if len(a) > 1 {
			v -= a[1] * math.Cos(2*math.Pi*fn/nn)
		}
		if len(a) > 2 {
			v += a[2] * math.Cos(4*math.Pi*fn/nn)
		}
		if len(a) > 3 {
			v -= a[3] * math.Cos(6*math.Pi*fn/nn)
		}
		output[n] = v
	}
}

func windowF32(output []float32, a []float64) {
	if len(a) < 1 || len(a) > 4 {
		panic("invalid window length " + strconv.Itoa(len(a)))
	}
	nn := float64(len(output) - 1)
	for n := range output {
		fn := float64(n)
		v := a[0]
		if len(a) > 1 {
			v -= a[1] * math.Cos(2*math.Pi*fn/nn)
		}
		if len(a) > 2 {
			v += a[2] * math.Cos(4*math.Pi*fn/nn)
		}
		if len(a) > 3 {
			v -= a[3] * math.Cos(6*math.Pi*fn/nn)
		}
		output[n] = float32(v)
	}
}
