package dspviz

import "math"

// The two frequency grids a response is measured on. They are a caller's
// choice rather than an option, because which one to use is a property of what
// the plot is for: a magnitude overview wants the logarithmic one, and a band
// metric wants the linear one so that every point in the band carries the same
// weight.

// LinearGrid returns n frequencies evenly spaced from low to high.
func LinearGrid(low, high float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	f := make([]float64, n)
	if n == 1 {
		f[0] = low
		return f
	}
	for i := range f {
		f[i] = low + (high-low)*float64(i)/float64(n-1)
	}
	return f
}

// LogGrid returns n frequencies spaced evenly in the logarithm from low to
// high, both of which must be positive. It is what a magnitude plot wants: a
// linear grid spends almost all of its points in the top octave.
func LogGrid(low, high float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	if !(low > 0) || !(high > 0) {
		// A caller that reached this with a zero has a Processor reporting a
		// zero rate; Analyze says so rather than getting here.
		panic("dspviz: a logarithmic grid needs positive bounds")
	}
	f := make([]float64, n)
	if n == 1 {
		f[0] = low
		return f
	}
	ll, lh := math.Log(low), math.Log(high)
	for i := range f {
		f[i] = math.Exp(ll + (lh-ll)*float64(i)/float64(n-1))
	}
	return f
}
