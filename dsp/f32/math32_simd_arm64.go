//go:build goexperiment.simd

package f32

import (
	"math"
	"simd/archsimd"

	"github.com/samuel/go-dsp/dsp/internal/view"
)

// Scale calculates dst[i] = src[i] * scale.
func Scale(dst, src []float32, scale float32) {
	// Load and store through *[N]float32, not the slice forms: those emit a
	// bounds check and re-derive the element pointer on every access, which the
	// compiler cannot hoist, and cost roughly 4x. Indexing off i is enough here;
	// amd64 has to advance the slices instead.
	n := min(len(src), len(dst))
	s := archsimd.BroadcastFloat32x4(scale)
	i := 0
	// Eight vectors per iteration. Four is measurably slower and sixteen
	// stops the compiler from unrolling cleanly.
	for m := n &^ 31; i < m; i += 32 {
		in := (*[32]float32)(src[i : i+32])
		out := (*[32]float32)(dst[i : i+32])
		a := archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4]))
		b := archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8]))
		c := archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12]))
		d := archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16]))
		e := archsimd.LoadFloat32x4Array((*[4]float32)(in[16:20]))
		f := archsimd.LoadFloat32x4Array((*[4]float32)(in[20:24]))
		g := archsimd.LoadFloat32x4Array((*[4]float32)(in[24:28]))
		h := archsimd.LoadFloat32x4Array((*[4]float32)(in[28:32]))
		a.Mul(s).StoreArray((*[4]float32)(out[0:4]))
		b.Mul(s).StoreArray((*[4]float32)(out[4:8]))
		c.Mul(s).StoreArray((*[4]float32)(out[8:12]))
		d.Mul(s).StoreArray((*[4]float32)(out[12:16]))
		e.Mul(s).StoreArray((*[4]float32)(out[16:20]))
		f.Mul(s).StoreArray((*[4]float32)(out[20:24]))
		g.Mul(s).StoreArray((*[4]float32)(out[24:28]))
		h.Mul(s).StoreArray((*[4]float32)(out[28:32]))
	}
	for m := n &^ 3; i < m; i += 4 {
		v := archsimd.LoadFloat32x4Array((*[4]float32)(src[i : i+4]))
		v.Mul(s).StoreArray((*[4]float32)(dst[i : i+4]))
	}
	for ; i < n; i++ {
		dst[i] = src[i] * scale
	}
}

// Max returns the maximum value of src, or -Inf for an empty slice. NaN makes the result unspecified.
func Max(src []float32) float32 {
	// Eight accumulators, sixteen vectors (64 floats) per iteration — 1.9x the
	// NEON assembly it replaced at 1K floats and 2.5x at 16K on an M2 Pro,
	// nearly all from the count: the assembly kept two, leaving it limited by
	// FMAX latency rather than the load ports. Sixteen accumulators cost 22% at
	// 64 floats; a 128-float block is a wash at 1K and 2.2x worse at 64. Only
	// sub-4K sizes see any of this; past that the loop is bandwidth-bound.
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x4(float32(math.Inf(-1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		m0 = m0.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])))
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])))
		m2 = m2.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])))
		m3 = m3.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])))
		m4 = m4.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[16:20])))
		m5 = m5.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[20:24])))
		m6 = m6.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[24:28])))
		m7 = m7.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[28:32])))
		m0 = m0.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[32:36])))
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[36:40])))
		m2 = m2.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[40:44])))
		m3 = m3.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[44:48])))
		m4 = m4.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[48:52])))
		m5 = m5.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[52:56])))
		m6 = m6.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[56:60])))
		m7 = m7.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[60:64])))
		src = src[64:]
	}
	// A four-vector step before the single-vector one, so a length that is not a
	// multiple of 64 does not fall through to a single dependent chain. Worth 8%
	// at 1023 floats and 16-25% between 16 and 80, and costs 8% at exactly 64.
	for len(src) >= 16 {
		in := (*[16]float32)(src[0:16])
		m0 = m0.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])))
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])))
		m2 = m2.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])))
		m3 = m3.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])))
		src = src[16:]
	}
	for len(src) >= 4 {
		m0 = m0.Max(archsimd.LoadFloat32x4Array((*[4]float32)(src[0:4])))
		src = src[4:]
	}
	// Fold the remainder in by re-reading the last full vector. Max is
	// idempotent, so re-accumulating elements cannot change the result, and the
	// scalar loop it replaces is a serial FMAXS chain worth 11-34% depending on
	// length. Only an input shorter than one vector still needs one.
	if len(src) > 0 && n >= 4 {
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(all[n-4 : n])))
		src = nil
	}
	// ReduceMax is one FMAXV. Finishing through memory instead costs 3-7% up to
	// 1023 floats; this core forwards the store cheaply, unlike amd64, which
	// open-codes the reduction as hmax4 to avoid a ~70-cycle stall.
	mx := m0.Max(m1).Max(m2.Max(m3)).Max(m4.Max(m5).Max(m6.Max(m7))).ReduceMax()
	for _, v := range src {
		mx = max(mx, v)
	}
	return mx
}

// Min returns the minimum value of src, or +Inf for an empty slice. NaN makes the result unspecified.
func Min(src []float32) float32 {
	// The mirror of Max above, down to the block sizes and the eight
	// accumulators. FMIN and FMAX cost the same and min is idempotent too, so
	// nothing is re-tuned; the reasoning is written out there.
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x4(float32(math.Inf(1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		m0 = m0.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])))
		m1 = m1.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])))
		m2 = m2.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])))
		m3 = m3.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])))
		m4 = m4.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[16:20])))
		m5 = m5.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[20:24])))
		m6 = m6.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[24:28])))
		m7 = m7.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[28:32])))
		m0 = m0.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[32:36])))
		m1 = m1.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[36:40])))
		m2 = m2.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[40:44])))
		m3 = m3.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[44:48])))
		m4 = m4.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[48:52])))
		m5 = m5.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[52:56])))
		m6 = m6.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[56:60])))
		m7 = m7.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[60:64])))
		src = src[64:]
	}
	for len(src) >= 16 {
		in := (*[16]float32)(src[0:16])
		m0 = m0.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])))
		m1 = m1.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])))
		m2 = m2.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])))
		m3 = m3.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])))
		src = src[16:]
	}
	for len(src) >= 4 {
		m0 = m0.Min(archsimd.LoadFloat32x4Array((*[4]float32)(src[0:4])))
		src = src[4:]
	}
	if len(src) > 0 && n >= 4 {
		m1 = m1.Min(archsimd.LoadFloat32x4Array((*[4]float32)(all[n-4 : n])))
		src = nil
	}
	mn := m0.Min(m1).Min(m2.Min(m3)).Min(m4.Min(m5).Min(m6.Min(m7))).ReduceMin()
	for _, v := range src {
		mn = min(mn, v)
	}
	return mn
}

// CAbs writes dst[i] = |src[i]| over min(len(src), len(dst)) elements.
func CAbs(dst []float32, src []complex64) {
	n := min(len(src), len(dst))
	in, out := view.C64ToF32(src)[:n*2], dst[:n]
	for len(out) >= 16 {
		i := (*[32]float32)(in[0:32])
		o := (*[16]float32)(out[0:16])
		absMag4((*[4]float32)(o[0:4]), (*[8]float32)(i[0:8]))
		absMag4((*[4]float32)(o[4:8]), (*[8]float32)(i[8:16]))
		absMag4((*[4]float32)(o[8:12]), (*[8]float32)(i[16:24]))
		absMag4((*[4]float32)(o[12:16]), (*[8]float32)(i[24:32]))
		in, out = in[32:], out[16:]
	}
	for len(out) >= 4 {
		absMag4((*[4]float32)(out[0:4]), (*[8]float32)(in[0:8]))
		in, out = in[8:], out[4:]
	}
	for i := range out {
		re, im := in[i*2], in[i*2+1]
		out[i] = float32(math.Sqrt(float64(float32(re*re) + float32(im*im))))
	}
}

// absMag4 writes the magnitudes of the four complex values in in.
//
// ConcatAddPairs is FADDP, adding adjacent lanes across the concatenation of
// its two operands, so squaring the two loads and folding them with one FADDP
// needs no deinterleave — unlike amd64, which shuffles the real and imaginary
// parts apart first. Squaring before the add also preserves the two roundings
// the scalar reference specifies; see vabsC64Scalar.
//
// Small enough to inline, keeping the loops above straight-line; check with
// -gcflags=-m after editing.
func absMag4(out *[4]float32, in *[8]float32) {
	a := archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4]))
	b := archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8]))
	a.Mul(a).ConcatAddPairs(b.Mul(b)).Sqrt().StoreArray(out)
}
