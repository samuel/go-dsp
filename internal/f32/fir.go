package f32

// The FIR and dot-product kernels behind dsp's FIR filters, decimators,
// interpolators and SincInterpolator.
//
// The references below define every result bit for bit, and a vector path has
// to reproduce them exactly. Each product is rounded to float32 before it is
// added, so arm64 cannot fuse a multiply and an add into an FMA that the other
// architectures do not perform.
//
// FIR vectorizes across outputs: each lane accumulates one output over the
// taps in order, which is the reference's order whatever the vector width.
// Dot sums into eight interleaved accumulators and combines them in a fixed
// tree, which a vector of four or eight lanes reproduces; the complex form
// does the same over each part, so its real and imaginary parts are exactly
// Dot of the real and imaginary parts.

// FIR writes dst[k] = sum over j of taps[j]*work[k*stride+len(taps)-1-j] for
// every k below len(dst), adding the products in order of j. work holds the
// len(taps)-1 samples of history in front of the first output's sample. It
// panics if taps is empty, stride is below 1, or work is shorter than
// (len(dst)-1)*stride+len(taps).
func FIR(dst, work, taps []float32, stride int) {
	checkFIR(len(dst), len(work), len(taps), stride)
	firF32(dst, work, taps, stride)
}

// CFIRReal is FIR over complex samples with real taps: FIR of the real and
// imaginary parts, separately.
func CFIRReal(dst, work []complex64, taps []float32, stride int) {
	checkFIR(len(dst), len(work), len(taps), stride)
	cfirReal(dst, work, taps, stride)
}

// CFIR is FIR over complex samples with complex taps. Each product is
// formed as in dsp.VMul, with its four partial products rounded.
func CFIR(dst, work, taps []complex64, stride int) {
	checkFIR(len(dst), len(work), len(taps), stride)
	cfir(dst, work, taps, stride)
}

// Dot returns the sum of x[i]*c[i] over len(c) elements, in the order described
// at the top of this file. It panics if x is shorter than c.
func Dot(x, c []float32) float32 {
	if len(x) < len(c) {
		panic("f32: Dot x is shorter than c")
	}
	return dotF32(x[:len(c)], c)
}

// CDotReal is Dot over complex samples with real coefficients: Dot of the real
// and imaginary parts, separately.
func CDotReal(x []complex64, c []float32) complex64 {
	if len(x) < len(c) {
		panic("f32: CDotReal x is shorter than c")
	}
	return cdotReal(x[:len(c)], c)
}

func checkFIR(dst, work, taps, stride int) {
	switch {
	case taps == 0:
		panic("f32: FIR needs at least one tap")
	case stride < 1:
		panic("f32: FIR stride must be at least 1")
	case dst > 0 && (work < taps || dst-1 > (work-taps)/stride):
		panic("f32: FIR work is shorter than (len(dst)-1)*stride+len(taps)")
	}
}

// firF32Go is the reference for FIR.
func firF32Go(dst, work, taps []float32, stride int) {
	last := len(taps) - 1
	for k := range dst {
		w := work[k*stride : k*stride+len(taps)]
		var sum float32
		for j, c := range taps {
			sum += float32(c * w[last-j])
		}
		dst[k] = sum
	}
}

// cfirRealGo is the reference for CFIRReal.
func cfirRealGo(dst, work []complex64, taps []float32, stride int) {
	last := len(taps) - 1
	for k := range dst {
		w := work[k*stride : k*stride+len(taps)]
		var re, im float32
		for j, c := range taps {
			v := w[last-j]
			re += float32(c * real(v))
			im += float32(c * imag(v))
		}
		dst[k] = complex(re, im)
	}
}

// cfirGo is the reference for CFIR. The real part of each product is the
// difference of two rounded products and the imaginary part their sum, and
// that is added to the accumulator.
func cfirGo(dst, work, taps []complex64, stride int) {
	last := len(taps) - 1
	for k := range dst {
		w := work[k*stride : k*stride+len(taps)]
		var re, im float32
		for j, c := range taps {
			v := w[last-j]
			cr, ci, vr, vi := real(c), imag(c), real(v), imag(v)
			re += float32(cr*vr) - float32(ci*vi)
			im += float32(cr*vi) + float32(ci*vr)
		}
		dst[k] = complex(re, im)
	}
}

// dotF32Go is the reference for Dot: eight accumulators over each block of
// eight, combined by combine8, then the remainder added in order.
func dotF32Go(x, c []float32) float32 {
	var acc [8]float32
	n := len(c) &^ 7
	for i := 0; i < n; i += 8 {
		for l := range acc {
			acc[l] += float32(x[i+l] * c[i+l])
		}
	}
	sum := combine8(&acc)
	for i := n; i < len(c); i++ {
		sum += float32(x[i] * c[i])
	}
	return sum
}

// cdotRealGo is the reference for CDotReal.
func cdotRealGo(x []complex64, c []float32) complex64 {
	var re, im [8]float32
	n := len(c) &^ 7
	for i := 0; i < n; i += 8 {
		for l := range re {
			v, m := x[i+l], c[i+l]
			re[l] += float32(real(v) * m)
			im[l] += float32(imag(v) * m)
		}
	}
	sr, si := combine8(&re), combine8(&im)
	for i := n; i < len(c); i++ {
		sr += float32(real(x[i]) * c[i])
		si += float32(imag(x[i]) * c[i])
	}
	return complex(sr, si)
}

// combine8 adds the upper four accumulators to the lower four, then the upper
// two of those to the lower two, then the last pair: the reduction a vector of
// eight or two of four performs.
func combine8(a *[8]float32) float32 {
	t0, t1, t2, t3 := a[0]+a[4], a[1]+a[5], a[2]+a[6], a[3]+a[7]
	return (t0 + t2) + (t1 + t3)
}
