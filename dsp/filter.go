package dsp

type IIRFilter struct {
	bCoef, aCoef []float64
	pIn, pOut    []float64
}

type ComplexIIRFilter32 struct {
	bCoef, aCoef []complex64
	pIn, pOut    []complex64
}

type ComplexIIRFilter struct {
	bCoef, aCoef []complex128
	pIn, pOut    []complex128
}

func NewIIRFilter(bCoef, aCoef []float64) *IIRFilter {
	if len(bCoef) != len(aCoef) || len(bCoef) == 0 {
		panic("IIR filter must have len(b)==len(a) and len(b) > 0")
	}
	for i, c := range bCoef {
		bCoef[i] = c / aCoef[0]
	}
	for i, c := range aCoef[1:] {
		aCoef[i+1] = c / aCoef[0]
	}
	return &IIRFilter{
		bCoef: bCoef,
		aCoef: aCoef,
		pIn:   make([]float64, len(bCoef)-1),
		pOut:  make([]float64, len(bCoef)-1),
	}
}

func NewComplexIIRFilter32(bCoef, aCoef []float32) *ComplexIIRFilter32 {
	if len(bCoef) != len(aCoef) || len(bCoef) == 0 {
		panic("IIR filter must have len(b)==len(a) and len(b) > 0")
	}
	for i, c := range bCoef {
		bCoef[i] = c / aCoef[0]
	}
	for i, c := range aCoef[1:] {
		aCoef[i+1] = c / aCoef[0]
	}
	return &ComplexIIRFilter32{
		bCoef: rtoc32(bCoef),
		aCoef: rtoc32(aCoef),
		pIn:   make([]complex64, len(bCoef)-1),
		pOut:  make([]complex64, len(bCoef)-1),
	}
}

func NewComplexIIRFilter(bCoef, aCoef []float64) *ComplexIIRFilter {
	if len(bCoef) != len(aCoef) || len(bCoef) == 0 {
		panic("IIR filter must have len(b)==len(a) and len(b) > 0")
	}
	for i, c := range bCoef {
		bCoef[i] = c / aCoef[0]
	}
	for i, c := range aCoef[1:] {
		aCoef[i+1] = c / aCoef[0]
	}
	return &ComplexIIRFilter{
		bCoef: rtoc(bCoef),
		aCoef: rtoc(aCoef),
		pIn:   make([]complex128, len(bCoef)-1),
		pOut:  make([]complex128, len(bCoef)-1),
	}
}

func (f *IIRFilter) Filter(input, output []float64) {
	for i, s := range input {
		sum := f.bCoef[0] * s
		for j, p := range f.pIn {
			sum += f.bCoef[j+1]*p - f.aCoef[j+1]*f.pOut[j]
		}
		for i := len(f.pIn) - 1; i > 0; i-- {
			f.pIn[i] = f.pIn[i-1]
			f.pOut[i] = f.pOut[i-1]
		}
		f.pIn[0] = s
		f.pOut[0] = sum
		output[i] = sum
	}
}

func (f *ComplexIIRFilter32) Filter(input, output []complex64) {
	for i, s := range input {
		sum := f.bCoef[0] * s
		for j, p := range f.pIn {
			sum += f.bCoef[j+1]*p - f.aCoef[j+1]*f.pOut[j]
		}
		for i := len(f.pIn) - 1; i > 0; i-- {
			f.pIn[i] = f.pIn[i-1]
			f.pOut[i] = f.pOut[i-1]
		}
		f.pIn[0] = s
		f.pOut[0] = sum
		output[i] = sum
	}
}

func (f *ComplexIIRFilter) Filter(input, output []complex128) {
	for i, s := range input {
		sum := f.bCoef[0] * s
		for j, p := range f.pIn {
			sum += f.bCoef[j+1]*p - f.aCoef[j+1]*f.pOut[j]
		}
		for i := len(f.pIn) - 1; i > 0; i-- {
			f.pIn[i] = f.pIn[i-1]
			f.pOut[i] = f.pOut[i-1]
		}
		f.pIn[0] = s
		f.pOut[0] = sum
		output[i] = sum
	}
}
