package f32

// CMulReal is the windowing kernel behind dsp.VCMulReal at complex64:
//
//	dst[i] = complex(real(src[i])*mul[i], imag(src[i])*mul[i])
//
// Scalar. A vector version needs each real value duplicated into both lanes,
// which is one shuffle.
func CMulReal(dst, src []complex64, mul []float32) {
	n := min(len(mul), len(src), len(dst))
	for i, v := range src[:n] {
		w := mul[i]
		dst[i] = complex(real(v)*w, imag(v)*w)
	}
}
