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
