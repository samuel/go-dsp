package dsp

// Float is the constraint for a real sample or coefficient, at either width.
// Exact types, not approximations: a defined type would satisfy ~float32 but
// would not match a []float32 case, making every dispatch switch in vector.go
// non-exhaustive.
type Float interface {
	float32 | float64
}

// Complex is the constraint for a complex sample or coefficient, at either
// width.
type Complex interface {
	complex64 | complex128
}

// Sample is anything the filters here can carry: a real or a complex sample, at
// either width.
type Sample interface {
	Float | Complex
}

// rtoc widens real coefficients to complex ones, which is what a filter with a
// real response needs to run over a complex signal.
func rtoc[C Complex, F Float](r []F) []C {
	c := make([]C, len(r))
	for i, v := range r {
		c[i] = C(complex(float64(v), 0))
	}
	return c
}
