package dsp

type LowPassDownsampleComplexFilter struct {
	Downsample int

	now       complex64
	prevIndex int
}

func (fi *LowPassDownsampleComplexFilter) Filter(samples []complex64) []complex64 {
	return lowPassDownsampleComplexFilterAsm(fi, samples)
}

func lowPassDownsampleComplexFilterAsm(fi *LowPassDownsampleComplexFilter, samples []complex64) []complex64

func lowPassDownsampleComplexFilter(fi *LowPassDownsampleComplexFilter, samples []complex64) []complex64 {
	i2 := 0
	// outputScale := 1.0 / float32(fi.Downsample)
	for i := 0; i < len(samples); i++ {
		fi.now += samples[i]
		fi.prevIndex++
		if fi.prevIndex < fi.Downsample {
			continue
		}
		samples[i2] = fi.now // complex(real(fi.now)*outputScale, imag(fi.now)*outputScale)
		fi.prevIndex = 0
		fi.now = 0
		i2++
	}
	return samples[:i2]
}

type LowPassDownsampleRationalFilter struct {
	Fast, Slow int

	sum       float32
	prevIndex int
}

func (fi *LowPassDownsampleRationalFilter) Filter(samples []float32) []float32 {
	return lowPassDownsampleRationalFilterAsm(fi, samples)
}

func lowPassDownsampleRationalFilterAsm(fi *LowPassDownsampleRationalFilter, samples []float32) []float32

func lowPassDownsampleRationalFilter(fi *LowPassDownsampleRationalFilter, samples []float32) []float32 {
	i2 := 0
	fastSlowRatio := float32(fi.Slow) / float32(fi.Fast)
	for i := 0; i < len(samples); i++ {
		fi.sum += samples[i]
		fi.prevIndex += fi.Slow
		if fi.prevIndex < fi.Fast {
			continue
		}
		samples[i2] = fi.sum * fastSlowRatio
		i2++
		fi.prevIndex -= fi.Fast
		fi.sum = 0.0
	}
	return samples[:i2]
}
