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

// setReal sets dst[k] to src[k]/div as a real value of T, for coefficients
// designed in float64. dst must hold len(src) values.
func setReal[T Sample](dst []T, src []float64, div float64) {
	dst = dst[:len(src)]
	switch d := any(dst).(type) {
	case []float32:
		for k, v := range src {
			d[k] = float32(v / div)
		}
	case []float64:
		for k, v := range src {
			d[k] = v / div
		}
	case []complex64:
		for k, v := range src {
			d[k] = complex(float32(v/div), 0)
		}
	case []complex128:
		for k, v := range src {
			d[k] = complex(v/div, 0)
		}
	default:
		panic("dsp: setReal: no case for this width")
	}
}

// realSample returns v as a T, with a zero imaginary part for a complex T.
func realSample[T Sample](v float64) T {
	var z T
	switch p := any(&z).(type) {
	case *float32:
		*p = float32(v)
	case *float64:
		*p = v
	case *complex64:
		*p = complex(float32(v), 0)
	case *complex128:
		*p = complex(v, 0)
	}
	return z
}
