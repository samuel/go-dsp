package f64

// The FIR and dot-product kernels at float64 and complex128, scalar on every
// architecture. They are defined exactly as f32's are: each product is rounded
// before it is added, FIR accumulates each output over the taps in order, and
// Dot sums into eight interleaved accumulators combined in a fixed tree.

// FIR writes dst[k] = sum over j of taps[j]*work[k*stride+len(taps)-1-j] for
// every k below len(dst), adding the products in order of j. work holds the
// len(taps)-1 samples of history in front of the first output's sample. It
// panics if taps is empty, stride is below 1, or work is shorter than
// (len(dst)-1)*stride+len(taps).
func FIR(dst, work, taps []float64, stride int) {
	checkFIR(len(dst), len(work), len(taps), stride)
	firF64(dst, work, taps, stride)
}

// CFIRReal is FIR over complex samples with real taps: FIR of the real and
// imaginary parts, separately.
func CFIRReal(dst, work []complex128, taps []float64, stride int) {
	checkFIR(len(dst), len(work), len(taps), stride)
	cfirReal(dst, work, taps, stride)
}

// CFIR is FIR over complex samples with complex taps. Each product is
// formed as in dsp.VMul, with its four partial products rounded.
func CFIR(dst, work, taps []complex128, stride int) {
	checkFIR(len(dst), len(work), len(taps), stride)
	cfir(dst, work, taps, stride)
}

// Dot returns the sum of x[i]*c[i] over len(c) elements, in the order described
// at the top of this file. It panics if x is shorter than c.
func Dot(x, c []float64) float64 {
	if len(x) < len(c) {
		panic("f64: Dot x is shorter than c")
	}
	return dotF64(x[:len(c)], c)
}

// CDotReal is Dot over complex samples with real coefficients: Dot of the real
// and imaginary parts, separately.
func CDotReal(x []complex128, c []float64) complex128 {
	if len(x) < len(c) {
		panic("f64: CDotReal x is shorter than c")
	}
	return cdotReal(x[:len(c)], c)
}

func checkFIR(dst, work, taps, stride int) {
	switch {
	case taps == 0:
		panic("f64: FIR needs at least one tap")
	case stride < 1:
		panic("f64: FIR stride must be at least 1")
	case dst > 0 && (work < taps || dst-1 > (work-taps)/stride):
		panic("f64: FIR work is shorter than (len(dst)-1)*stride+len(taps)")
	}
}

// firF64 is FIR without the checks.
func firF64(dst, work, taps []float64, stride int) {
	last := len(taps) - 1
	for k := range dst {
		w := work[k*stride : k*stride+len(taps)]
		var sum float64
		for j, c := range taps {
			sum += float64(c * w[last-j])
		}
		dst[k] = sum
	}
}

// cfirReal is CFIRReal without the checks.
func cfirReal(dst, work []complex128, taps []float64, stride int) {
	last := len(taps) - 1
	for k := range dst {
		w := work[k*stride : k*stride+len(taps)]
		var re, im float64
		for j, c := range taps {
			v := w[last-j]
			re += float64(c * real(v))
			im += float64(c * imag(v))
		}
		dst[k] = complex(re, im)
	}
}

// cfir is CFIR without the checks. The real part of each product is the
// difference of two rounded products and the imaginary part their sum, and
// that is added to the accumulator.
func cfir(dst, work, taps []complex128, stride int) {
	last := len(taps) - 1
	for k := range dst {
		w := work[k*stride : k*stride+len(taps)]
		var re, im float64
		for j, c := range taps {
			v := w[last-j]
			cr, ci, vr, vi := real(c), imag(c), real(v), imag(v)
			re += float64(cr*vr) - float64(ci*vi)
			im += float64(cr*vi) + float64(ci*vr)
		}
		dst[k] = complex(re, im)
	}
}

// dotF64 is Dot without the check: eight accumulators over each block of
// eight, combined by combine8, then the remainder added in order.
func dotF64(x, c []float64) float64 {
	var acc [8]float64
	n := len(c) &^ 7
	for i := 0; i < n; i += 8 {
		for l := range acc {
			acc[l] += float64(x[i+l] * c[i+l])
		}
	}
	sum := combine8(&acc)
	for i := n; i < len(c); i++ {
		sum += float64(x[i] * c[i])
	}
	return sum
}

// cdotReal is CDotReal without the check.
func cdotReal(x []complex128, c []float64) complex128 {
	var re, im [8]float64
	n := len(c) &^ 7
	for i := 0; i < n; i += 8 {
		for l := range re {
			v, m := x[i+l], c[i+l]
			re[l] += float64(real(v) * m)
			im[l] += float64(imag(v) * m)
		}
	}
	sr, si := combine8(&re), combine8(&im)
	for i := n; i < len(c); i++ {
		sr += float64(real(x[i]) * c[i])
		si += float64(imag(x[i]) * c[i])
	}
	return complex(sr, si)
}

// combine8 adds the upper four accumulators to the lower four, then the upper
// two of those to the lower two, then the last pair: the reduction a vector of
// eight or two of four performs.
func combine8(a *[8]float64) float64 {
	t0, t1, t2, t3 := a[0]+a[4], a[1]+a[5], a[2]+a[6], a[3]+a[7]
	return (t0 + t2) + (t1 + t3)
}
