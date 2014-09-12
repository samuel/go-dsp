package sdr

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

type Rotate90Filter struct {
}

func (fi *Rotate90Filter) Filter(samples []complex64) []complex64 {
	return rotate90FilterAsm(fi, samples)
}

func rotate90FilterAsm(fi *Rotate90Filter, samples []complex64) []complex64

func rotate90Filter(fi *Rotate90Filter, samples []complex64) []complex64 {
	for i := 0; i < len(samples); i += 4 {
		samples[i+1] = complex(-imag(samples[i+1]), real(samples[i+1]))
		samples[i+2] = -samples[i+2]
		samples[i+3] = complex(imag(samples[i+3]), -real(samples[i+3]))
	}
	return samples
}

type I32Rotate90Filter struct {
}

func (fi *I32Rotate90Filter) Filter(samples []int32) []int32 {
	return i32Rotate90FilterAsm(fi, samples)
}

func i32Rotate90FilterAsm(fi *I32Rotate90Filter, samples []int32) []int32

func i32Rotate90Filter(fi *I32Rotate90Filter, samples []int32) []int32 {
	for i := 0; i < len(samples); i += 8 {
		samples[i+2], samples[i+3] = -samples[i+3], samples[i+2]
		samples[i+4] = -samples[i+4]
		samples[i+5] = -samples[i+5]
		samples[i+6], samples[i+7] = samples[i+7], -samples[i+6]
	}
	return samples
}
