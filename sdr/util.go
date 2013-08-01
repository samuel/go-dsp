package sdr

import (
	"math"
	"math/cmplx"
)

type ComplexSource interface {
	Source() ([]complex64, error)
}

type RealSink interface {
	Sink([]float32) error
}

type ComplexFilter interface {
	Filter([]complex64) ([]complex64, error)
}

type Demodulator interface {
	Demodulate(input []complex64, output []float32) (int, error)
}

/////////// native complex128

type Rotate90Filter struct {
	currentAngle int
}

func (fi *Rotate90Filter) Filter(samples []complex64) ([]complex64, error) {
	for i := 0; i < len(samples); i++ {
		switch fi.currentAngle {
		case 0:
			// noop
		case 1:
			samples[i] = complex(-imag(samples[i+1]), real(samples[i+1]))
		case 2:
			samples[i] = -samples[i+2]
		case 3:
			samples[i] = complex(imag(samples[i+3]), -real(samples[i+3]))
		}
		fi.currentAngle = (fi.currentAngle + 1) & 3
	}
	return samples, nil
}

type LowPassDownsampleComplexFilter struct {
	Downsample int

	now       complex64
	prevIndex int
}

func (fi *LowPassDownsampleComplexFilter) Filter(samples []complex64) ([]complex64, error) {
	i2 := 0
	for i := 0; i < len(samples); i++ {
		fi.now += samples[i]
		fi.prevIndex++
		if fi.prevIndex < fi.Downsample {
			continue
		}
		samples[i2] = fi.now // * outputScale
		fi.prevIndex = 0
		fi.now = 0
		i2++
	}
	return samples[:i2], nil
}

func PolarDiscriminator(a, b complex128) float64 {
	return cmplx.Phase(a*cmplx.Conj(b)) / math.Pi
}

func PolarDiscriminator32(a, b complex64) float32 {
	return Phase32(a*Conj32(b)) / math.Pi
}

type FMDemodFilter struct {
	pre complex64
}

func (fi *FMDemodFilter) Demodulate(input []complex64, output []float32) (int, error) {
	for i := 0; i < len(input); i++ {
		pcm := PolarDiscriminator32(input[i], fi.pre)
		fi.pre = input[i]
		output[i] = pcm
	}
	return len(input), nil
}

type LowPassDownsampleFilter struct {
	Downsample int
}

func (fi *LowPassDownsampleFilter) Filter(samples []float32) ([]float32, error) {
	i := 0
	for i < len(samples) {
		sum := float32(0.0)
		for i2 := 0; i2 < fi.Downsample; i2++ {
			sum += samples[i+i2]
		}
		samples[i/fi.Downsample] = sum // /step
		i += fi.Downsample
	}
	samples[i/fi.Downsample+1] = samples[i/fi.Downsample]
	return samples[:len(samples)/fi.Downsample], nil
}

type LowPassDownsampleRationalFilter struct {
	Fast, Slow int

	nowLPR       float32
	prevLPRIndex int
}

func (fi *LowPassDownsampleRationalFilter) Filter(samples []float32) ([]float32, error) {
	i2 := 0
	fastSlowRatio := float32(fi.Fast) / float32(fi.Slow)
	for i := 0; i < len(samples); i++ {
		fi.nowLPR += samples[i]
		fi.prevLPRIndex += fi.Slow
		if fi.prevLPRIndex < fi.Fast {
			continue
		}
		samples[i2] = fi.nowLPR / fastSlowRatio
		fi.prevLPRIndex -= fi.Fast
		fi.nowLPR = 0
		i2++
	}
	return samples[:i2], nil
}
