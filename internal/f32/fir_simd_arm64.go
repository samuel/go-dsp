//go:build goexperiment.simd

package f32

import (
	"simd/archsimd"

	"samuelks.com/dsp/internal/view"
)

// The archsimd forms of the NEON kernels in fir_nosimd_arm64.s. Every product
// is a Mul followed by an Add: the compiler fuses only an explicit MulAdd, and
// a fused product would not match the references.
//
// The FIR leaves vectorize across outputs, which needs consecutive outputs to
// read consecutive samples; a stride above 1 (a decimator) runs the reference.

func firF32(dst, work, taps []float32, stride int) {
	if stride != 1 {
		firF32Go(dst, work, taps, stride)
		return
	}
	firSIMD(dst, work, taps, 1)
}

func cfirReal(dst, work []complex64, taps []float32, stride int) {
	if stride != 1 {
		cfirRealGo(dst, work, taps, stride)
		return
	}
	firSIMD(view.C64ToF32(dst), view.C64ToF32(work), taps, 2)
}

func cfir(dst, work, taps []complex64, stride int) {
	if stride != 1 {
		cfirGo(dst, work, taps, stride)
		return
	}
	cfirSIMD(dst, work, taps)
}

// firSIMD is FIR over len(dst) outputs with the taps reaching back step
// elements at a time; step 2 runs CFIRReal over the interleaved floats.
//
// Lane l of a block accumulates output k+l over the taps in order. Thirty-two
// outputs (eight accumulators) per block keep enough FADDs in flight to hide
// their latency; eight, four and a scalar loop finish the rest.
func firSIMD(dst, work, taps []float32, step int) {
	span := (len(taps) - 1) * step
	n := len(dst)
	k := 0
	for ; k+32 <= n; k += 32 {
		w := work[k : k+span+32]
		var a0, a1, a2, a3, a4, a5, a6, a7 archsimd.Float32x4
		off := span
		for _, c := range taps {
			b := archsimd.BroadcastFloat32x4(c)
			in := (*[32]float32)(w[off : off+32])
			a0 = a0.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).Mul(b))
			a1 = a1.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).Mul(b))
			a2 = a2.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])).Mul(b))
			a3 = a3.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])).Mul(b))
			a4 = a4.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[16:20])).Mul(b))
			a5 = a5.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[20:24])).Mul(b))
			a6 = a6.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[24:28])).Mul(b))
			a7 = a7.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[28:32])).Mul(b))
			off -= step
		}
		out := (*[32]float32)(dst[k : k+32])
		a0.StoreArray((*[4]float32)(out[0:4]))
		a1.StoreArray((*[4]float32)(out[4:8]))
		a2.StoreArray((*[4]float32)(out[8:12]))
		a3.StoreArray((*[4]float32)(out[12:16]))
		a4.StoreArray((*[4]float32)(out[16:20]))
		a5.StoreArray((*[4]float32)(out[20:24]))
		a6.StoreArray((*[4]float32)(out[24:28]))
		a7.StoreArray((*[4]float32)(out[28:32]))
	}
	for ; k+8 <= n; k += 8 {
		w := work[k : k+span+8]
		var a0, a1 archsimd.Float32x4
		off := span
		for _, c := range taps {
			b := archsimd.BroadcastFloat32x4(c)
			in := (*[8]float32)(w[off : off+8])
			a0 = a0.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).Mul(b))
			a1 = a1.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).Mul(b))
			off -= step
		}
		out := (*[8]float32)(dst[k : k+8])
		a0.StoreArray((*[4]float32)(out[0:4]))
		a1.StoreArray((*[4]float32)(out[4:8]))
	}
	if k+4 <= n {
		w := work[k : k+span+4]
		var a archsimd.Float32x4
		off := span
		for _, c := range taps {
			a = a.Add(archsimd.LoadFloat32x4Array((*[4]float32)(w[off : off+4])).Mul(archsimd.BroadcastFloat32x4(c)))
			off -= step
		}
		a.StoreArray((*[4]float32)(dst[k : k+4]))
		k += 4
	}
	for ; k < n; k++ {
		w := work[k : k+span+1]
		var sum float32
		off := span
		for _, c := range taps {
			sum += float32(c * w[off])
			off -= step
		}
		dst[k] = sum
	}
}

// cfirSIMD is CFIR with a stride of 1. ConcatEven and ConcatOdd split four
// complex samples into a vector of real parts and one of imaginary parts, and
// InterleaveLo and InterleaveHi put the accumulators back together, so each
// tap costs four products, their difference and sum, and two accumulations
// per four outputs. Sub is the reference's subtraction exactly.
//
// Sixteen outputs (eight accumulators) per block, then four, then a scalar
// loop.
func cfirSIMD(dst, work, taps []complex64) {
	d, w := view.C64ToF32(dst), view.C64ToF32(work)
	span := (len(taps) - 1) * 2
	n := len(d)
	k := 0
	for ; k+32 <= n; k += 32 {
		win := w[k : k+span+32]
		var r0, i0, r1, i1, r2, i2, r3, i3 archsimd.Float32x4
		off := span
		for _, c := range taps {
			cr := archsimd.BroadcastFloat32x4(real(c))
			ci := archsimd.BroadcastFloat32x4(imag(c))
			in := (*[32]float32)(win[off : off+32])
			r0, i0 = cmulAcc(r0, i0, (*[8]float32)(in[0:8]), cr, ci)
			r1, i1 = cmulAcc(r1, i1, (*[8]float32)(in[8:16]), cr, ci)
			r2, i2 = cmulAcc(r2, i2, (*[8]float32)(in[16:24]), cr, ci)
			r3, i3 = cmulAcc(r3, i3, (*[8]float32)(in[24:32]), cr, ci)
			off -= 2
		}
		out := (*[32]float32)(d[k : k+32])
		storeInterleaved((*[8]float32)(out[0:8]), r0, i0)
		storeInterleaved((*[8]float32)(out[8:16]), r1, i1)
		storeInterleaved((*[8]float32)(out[16:24]), r2, i2)
		storeInterleaved((*[8]float32)(out[24:32]), r3, i3)
	}
	for ; k+8 <= n; k += 8 {
		win := w[k : k+span+8]
		var re, im archsimd.Float32x4
		off := span
		for _, c := range taps {
			cr := archsimd.BroadcastFloat32x4(real(c))
			ci := archsimd.BroadcastFloat32x4(imag(c))
			re, im = cmulAcc(re, im, (*[8]float32)(win[off:off+8]), cr, ci)
			off -= 2
		}
		storeInterleaved((*[8]float32)(d[k:k+8]), re, im)
	}
	for ; k < n; k += 2 {
		win := w[k : k+span+2]
		var re, im float32
		off := span
		for _, c := range taps {
			cr, ci, vr, vi := real(c), imag(c), win[off], win[off+1]
			re += float32(cr*vr) - float32(ci*vi)
			im += float32(cr*vi) + float32(ci*vr)
			off -= 2
		}
		d[k], d[k+1] = re, im
	}
}

// cmulAcc adds the four complex products of the samples in in and the tap
// (cr, ci) to the accumulators re and im. Small enough to inline.
func cmulAcc(re, im archsimd.Float32x4, in *[8]float32, cr, ci archsimd.Float32x4) (archsimd.Float32x4, archsimd.Float32x4) {
	a := archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).ToBits()
	b := archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).ToBits()
	vr := a.ConcatEven(b).BitsToFloat32()
	vi := a.ConcatOdd(b).BitsToFloat32()
	re = re.Add(vr.Mul(cr).Sub(vi.Mul(ci)))
	im = im.Add(vi.Mul(cr).Add(vr.Mul(ci)))
	return re, im
}

// storeInterleaved writes four complex values from their real and imaginary
// parts.
func storeInterleaved(out *[8]float32, re, im archsimd.Float32x4) {
	r, i := re.ToBits(), im.ToBits()
	r.InterleaveLo(i).BitsToFloat32().StoreArray((*[4]float32)(out[0:4]))
	r.InterleaveHi(i).BitsToFloat32().StoreArray((*[4]float32)(out[4:8]))
}

// dotF32 keeps the eight accumulators of dotF32Go in the lanes of a (0-3) and
// b (4-7) and finishes with combine8's tree. The lanes are serial chains, so a
// longer block would change the result.
func dotF32(x, c []float32) float32 {
	x = x[:len(c)]
	var a, b archsimd.Float32x4
	i := 0
	for ; i+8 <= len(c); i += 8 {
		xs := (*[8]float32)(x[i : i+8])
		cs := (*[8]float32)(c[i : i+8])
		a = a.Add(archsimd.LoadFloat32x4Array((*[4]float32)(xs[0:4])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cs[0:4]))))
		b = b.Add(archsimd.LoadFloat32x4Array((*[4]float32)(xs[4:8])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cs[4:8]))))
	}
	sum := combine8x4(a, b)
	for ; i < len(c); i++ {
		sum += float32(x[i] * c[i])
	}
	return sum
}

// cdotReal is dotF32 over each part. The samples stay interleaved and the
// coefficients are duplicated to match, [c0 c0 c1 c1 ...], so q0-q3 hold the
// real and imaginary accumulators of samples 0-1, 2-3, 4-5 and 6-7 of each
// block. q0+q2 and q1+q3 are combine8's first step for both parts at once,
// their sum the second, and the two halves of that the last.
func cdotReal(x []complex64, c []float32) complex64 {
	xf := view.C64ToF32(x[:len(c)])
	var q0, q1, q2, q3 archsimd.Float32x4
	for len(c) >= 8 && len(xf) >= 16 {
		xs := (*[16]float32)(xf[:16])
		cs := (*[8]float32)(c[:8])
		xf, c = xf[16:], c[8:]
		ca := archsimd.LoadFloat32x4Array((*[4]float32)(cs[0:4])).ToBits()
		cb := archsimd.LoadFloat32x4Array((*[4]float32)(cs[4:8])).ToBits()
		q0 = q0.Add(archsimd.LoadFloat32x4Array((*[4]float32)(xs[0:4])).Mul(ca.InterleaveLo(ca).BitsToFloat32()))
		q1 = q1.Add(archsimd.LoadFloat32x4Array((*[4]float32)(xs[4:8])).Mul(ca.InterleaveHi(ca).BitsToFloat32()))
		q2 = q2.Add(archsimd.LoadFloat32x4Array((*[4]float32)(xs[8:12])).Mul(cb.InterleaveLo(cb).BitsToFloat32()))
		q3 = q3.Add(archsimd.LoadFloat32x4Array((*[4]float32)(xs[12:16])).Mul(cb.InterleaveHi(cb).BitsToFloat32()))
	}
	s := q0.Add(q2).Add(q1.Add(q3))
	s = s.Add(s.HiToLo())
	sr, si := s.GetElem(0), s.GetElem(1)
	for i, m := range c {
		sr += float32(xf[2*i] * m)
		si += float32(xf[2*i+1] * m)
	}
	return complex(sr, si)
}

// combine8x4 is combine8 over accumulators 0-3 in a and 4-7 in b: t = a+b,
// then t plus its upper pair moved down, then the two low lanes. HiToLo
// zeroes the upper half, which the last step does not read.
func combine8x4(a, b archsimd.Float32x4) float32 {
	t := a.Add(b)
	return t.Add(t.HiToLo()).ConcatAddPairs(t).GetElem(0)
}
