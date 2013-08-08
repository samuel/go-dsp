package sdr

type LowPassDownsampleComplexFilter struct {
	Downsample int

	now       complex64
	prevIndex int
}

func (fi *LowPassDownsampleComplexFilter) Filter(samples []complex64) ([]complex64, error) {
	return lowPassDownsampleComplexFilterAsm(fi, samples)
}

func lowPassDownsampleComplexFilterAsm(fi *LowPassDownsampleComplexFilter, samples []complex64) ([]complex64, error)

func lowPassDownsampleComplexFilter(fi *LowPassDownsampleComplexFilter, samples []complex64) ([]complex64, error) {
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

type LowPassDownsampleRationalFilter struct {
	Fast, Slow int

	sum       float32
	prevIndex int
}

func (fi *LowPassDownsampleRationalFilter) Filter(samples []float32) ([]float32, error) {
	return lowPassDownsampleRationalFilterAsm(fi, samples)
}

func lowPassDownsampleRationalFilterAsm(fi *LowPassDownsampleRationalFilter, samples []float32) ([]float32, error)

func lowPassDownsampleRationalFilter(fi *LowPassDownsampleRationalFilter, samples []float32) ([]float32, error) {
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
	return samples[:i2], nil
}
