package dsp

import "math"

var (
	BlackmanFreqCoeff = []float64{0.16 / 4, -1.0 / 4, (1 - 0.16) / 2, -1.0 / 4, 0.16 / 4}
	HammingFreqCoeff  = []float64{(0.53836 - 1) / 2, 0.53836, (0.53836 - 1) / 2}
	HanningFreqCoeff  = []float64{-0.25, 0.5, -0.25}

	BlackmanFreqCoeff32 = []float32{0.16 / 4, -1.0 / 4, (1 - 0.16) / 2, -1.0 / 4, 0.16 / 4}
	HammingFreqCoeff32  = []float32{(0.53836 - 1) / 2, 0.53836, (0.53836 - 1) / 2}
	HanningFreqCoeff32  = []float32{-0.25, 0.5, -0.25}
)

func TriangleWindow(output []float64) {
	for n := 0; n < len(output); n++ {
		output[n] = 1 - math.Abs((float64(n)-float64(len(output)-1)/2.0)/(float64(len(output)+1)/2.0))
	}
}

func TriangleWindowF32(output []float32) {
	for n := 0; n < len(output); n++ {
		output[n] = float32(1 - math.Abs((float64(n)-float64(len(output)-1)/2.0)/(float64(len(output)+1)/2.0)))
	}
}

func HammingWindow(output []float64) {
	a := 0.53836
	b := 1 - a
	for n := 0; n < len(output); n++ {
		output[n] = a - b*math.Cos(2*math.Pi*float64(n)/float64(len(output)-1))
	}
}

func HammingWindowF32(output []float32) {
	a := 0.53836
	b := 1 - a
	for n := 0; n < len(output); n++ {
		output[n] = float32(a - b*math.Cos(2*math.Pi*float64(n)/float64(len(output)-1)))
	}
}

func HanningWindow(output []float64) {
	for n := 0; n < len(output); n++ {
		output[n] = 0.5 * (1 - math.Cos(2*math.Pi*float64(n)/float64(len(output)-1)))
	}
}

func HanningWindowF32(output []float32) {
	for n := 0; n < len(output); n++ {
		output[n] = float32(0.5 * (1 - math.Cos(2*math.Pi*float64(n)/float64(len(output)-1))))
	}
}

func BlackmanWindow(output []float64) {
	a := 0.16
	a0 := (1 - a) / 2
	a1 := 1.0 / 2.0
	a2 := a / 2
	for n := 0; n < len(output); n++ {
		output[n] = a0 - a1*math.Cos(2*math.Pi*float64(n)/float64(len(output)-1)) + a2*math.Cos(4*math.Pi*float64(n)/float64(len(output)-1))
	}
}
