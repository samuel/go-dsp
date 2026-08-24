//go:build goexperiment.simd

package f32

import (
	"math"
	"simd/archsimd"

	"github.com/samuel/go-dsp/dsp/internal/view"
)

// Scale calculates dst[i] = src[i] * scale.
func Scale(dst, src []float32, scale float32) {
	if !haveSIMD() {
		// Pre-AVX CPU: archsimd compiles even its 128-bit operations to VEX
		// encodings, so none of the paths below can run. See simdWidth.
		vscaleF32Asm(dst, src, scale)
		return
	}
	n := min(len(src), len(dst))
	switch {
	case simdWidth >= 512:
		vscaleF32x16(dst[:n], src[:n], scale)
	case simdWidth >= 256:
		vscaleF32x8(dst[:n], src[:n], scale)
	default:
		vscaleF32x4(dst[:n], src[:n], scale)
	}
	// VZEROUPPER. Besides the AVX-to-SSE transition penalty, a dirty upper
	// state holds the core at the AVX-512 frequency license: the runtime's
	// asyncPreempt saves all 32 ZMM registers whenever the CPU has AVX-512,
	// gated on the feature bit rather than on what it preempted. Worth up to
	// 60% on a Xeon Silver 4210, and too cheap to measure even on 4 elements.
	archsimd.ClearAVXUpperBits()
}

// Measured on a Xeon Silver 4210, three loop structures matter:
//
//   - Array-pointer loads and stores (*[N]float32), not the slice forms, which
//     re-derive the element pointer and bounds-check on every access (~4x).
//   - Advancing src and dst rather than indexing a base: amd64 neither hoists
//     the checks nor commons up base+i*4, emitting a pair of LEAQs per access.
//     Worth ~15%. (arm64 does not need this.)
//   - Large blocks: each reslice is a six-instruction guarded advance and dst's
//     bounds check cannot be elided, so a 4-vector body is 27 fused-domain uops
//     per 32 floats against 16 for equivalent assembly.
//
// unsafe.Pointer arithmetic would drop the guarded advance for another ~8% and
// is deliberately not used: these functions accept mismatched src and dst
// lengths, so the bounds check is what keeps that a panic, not corruption.
//
// Only L1/L2-resident sizes show any of this. Past that the loops are
// bandwidth-bound and every variant converges.

// Four 512-bit vectors (64 floats) per iteration. Eight is faster in L1 and
// ~8% slower from 8K floats up; four assumes streaming buffers dominate.
// Unmeasured on a CPU that selects this path (see avx512Downclocks).
func vscaleF32x16(dst, src []float32, scale float32) {
	s := archsimd.BroadcastFloat32x16(scale)
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		out := (*[64]float32)(dst[0:64])
		a := archsimd.LoadFloat32x16Array((*[16]float32)(in[0:16]))
		b := archsimd.LoadFloat32x16Array((*[16]float32)(in[16:32]))
		c := archsimd.LoadFloat32x16Array((*[16]float32)(in[32:48]))
		d := archsimd.LoadFloat32x16Array((*[16]float32)(in[48:64]))
		a.Mul(s).StoreArray((*[16]float32)(out[0:16]))
		b.Mul(s).StoreArray((*[16]float32)(out[16:32]))
		c.Mul(s).StoreArray((*[16]float32)(out[32:48]))
		d.Mul(s).StoreArray((*[16]float32)(out[48:64]))
		src, dst = src[64:], dst[64:]
	}
	for len(src) >= 16 {
		v := archsimd.LoadFloat32x16Array((*[16]float32)(src[0:16]))
		v.Mul(s).StoreArray((*[16]float32)(dst[0:16]))
		src, dst = src[16:], dst[16:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

// Eight 256-bit vectors (64 floats) per iteration, the width most hardware
// selects; against four, 20-31% up to 4K floats, level beyond. Compare unroll
// factors with both bodies in one test binary — separate binaries differ more
// from code layout than from the unroll.
func vscaleF32x8(dst, src []float32, scale float32) {
	s := archsimd.BroadcastFloat32x8(scale)
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		out := (*[64]float32)(dst[0:64])
		a := archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8]))
		b := archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16]))
		c := archsimd.LoadFloat32x8Array((*[8]float32)(in[16:24]))
		d := archsimd.LoadFloat32x8Array((*[8]float32)(in[24:32]))
		e := archsimd.LoadFloat32x8Array((*[8]float32)(in[32:40]))
		f := archsimd.LoadFloat32x8Array((*[8]float32)(in[40:48]))
		g := archsimd.LoadFloat32x8Array((*[8]float32)(in[48:56]))
		h := archsimd.LoadFloat32x8Array((*[8]float32)(in[56:64]))
		a.Mul(s).StoreArray((*[8]float32)(out[0:8]))
		b.Mul(s).StoreArray((*[8]float32)(out[8:16]))
		c.Mul(s).StoreArray((*[8]float32)(out[16:24]))
		d.Mul(s).StoreArray((*[8]float32)(out[24:32]))
		e.Mul(s).StoreArray((*[8]float32)(out[32:40]))
		f.Mul(s).StoreArray((*[8]float32)(out[40:48]))
		g.Mul(s).StoreArray((*[8]float32)(out[48:56]))
		h.Mul(s).StoreArray((*[8]float32)(out[56:64]))
		src, dst = src[64:], dst[64:]
	}
	for len(src) >= 8 {
		v := archsimd.LoadFloat32x8Array((*[8]float32)(src[0:8]))
		v.Mul(s).StoreArray((*[8]float32)(dst[0:8]))
		src, dst = src[8:], dst[8:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

// Eight 128-bit vectors (32 floats) per iteration. Sixteen is 25% worse at 16K
// floats, four is worse everywhere.
func vscaleF32x4(dst, src []float32, scale float32) {
	s := archsimd.BroadcastFloat32x4(scale)
	for len(src) >= 32 {
		in := (*[32]float32)(src[0:32])
		out := (*[32]float32)(dst[0:32])
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
		src, dst = src[32:], dst[32:]
	}
	for len(src) >= 4 {
		v := archsimd.LoadFloat32x4Array((*[4]float32)(src[0:4]))
		v.Mul(s).StoreArray((*[4]float32)(dst[0:4]))
		src, dst = src[4:], dst[4:]
	}
	for i, v := range src {
		dst[i] = v * scale
	}
}

// Max returns the maximum value of src, or -Inf for an empty slice. NaN makes the result unspecified.
func Max(src []float32) float32 {
	if !haveSIMD() {
		// Pre-AVX CPU; see Scale.
		return vmaxF32Asm(src)
	}
	var mx float32
	switch {
	case simdWidth >= 512:
		mx = vmaxF32x16(src)
	case simdWidth >= 256:
		mx = vmaxF32x8(src)
	default:
		mx = vmaxF32x4(src)
	}
	archsimd.ClearAVXUpperBits() // see Scale
	return mx
}

// The loops below take Scale's loop structure — array-pointer loads, advancing slices —
// plus four things from this being a reduction:
//
//   - Eight independent accumulators, to cover the 4-cycle latency of VMAXPS
//     while the load ports keep up. Two cost 190% at 4K floats, four 62%.
//   - A four-vector step between the main block and the single-vector loop, so
//     a length that is not a multiple of the block does not fall to one
//     dependent chain.
//   - A register-resident horizontal reduction (hmax4). StoreArray with a
//     scalar max is a 16-byte store feeding four 4-byte loads, a fixed ~70-cycle
//     store-forwarding stall that made a 64-float max 3.2x slower than it
//     needed to be.
//   - A remainder folded in by re-reading the last full vector. Max is
//     idempotent, so re-accumulating elements is harmless, and Go's max()
//     builtin must honor NaN and signed zero, so it is branchy rather than one
//     MAXSS — at 1023 floats a scalar tail cost 54ns on top of a 46ns loop.
//     Only inputs shorter than one vector still need one.
//
// The accumulator count and the intermediate step carry across architectures;
// the block sizes do not, and these have not been measured on amd64.

// hmax4 returns the maximum of v's four elements, in registers
// (VSHUFPS/VMAXPS/VPEXTRD).
func hmax4(v archsimd.Float32x4) float32 {
	v = v.Max(v.ConcatPermuteScalars(2, 3, 2, 3, v))
	v = v.Max(v.ConcatPermuteScalars(1, 1, 1, 1, v))
	return v.GetElem(0)
}

// Eight 512-bit vectors (128 floats) per iteration, one accumulator each,
// saturating a core issuing two VMAXPS per cycle at four cycles of latency.
// Untuned, since no available CPU selects this path.
func vmaxF32x16(src []float32) float32 {
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x16(float32(math.Inf(-1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 128 {
		in := (*[128]float32)(src[0:128])
		m0 = m0.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[0:16])))
		m1 = m1.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[16:32])))
		m2 = m2.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[32:48])))
		m3 = m3.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[48:64])))
		m4 = m4.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[64:80])))
		m5 = m5.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[80:96])))
		m6 = m6.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[96:112])))
		m7 = m7.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[112:128])))
		src = src[128:]
	}
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		m0 = m0.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[0:16])))
		m1 = m1.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[16:32])))
		m2 = m2.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[32:48])))
		m3 = m3.Max(archsimd.LoadFloat32x16Array((*[16]float32)(in[48:64])))
		src = src[64:]
	}
	for len(src) >= 16 {
		m0 = m0.Max(archsimd.LoadFloat32x16Array((*[16]float32)(src[0:16])))
		src = src[16:]
	}
	if len(src) > 0 && n >= 16 {
		m1 = m1.Max(archsimd.LoadFloat32x16Array((*[16]float32)(all[n-16 : n])))
		src = nil
	}
	v16 := m0.Max(m1).Max(m2.Max(m3)).Max(m4.Max(m5).Max(m6.Max(m7)))
	v8 := v16.GetLo().Max(v16.GetHi())
	mx := hmax4(v8.GetLo().Max(v8.GetHi()))
	for _, v := range src {
		mx = max(mx, v)
	}
	return mx
}

// Eight 256-bit vectors (64 floats) per iteration, one accumulator each, the
// width most hardware selects. Sixteen is a wash.
func vmaxF32x8(src []float32) float32 {
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x8(float32(math.Inf(-1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		m0 = m0.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8])))
		m1 = m1.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16])))
		m2 = m2.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[16:24])))
		m3 = m3.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[24:32])))
		m4 = m4.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[32:40])))
		m5 = m5.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[40:48])))
		m6 = m6.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[48:56])))
		m7 = m7.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[56:64])))
		src = src[64:]
	}
	for len(src) >= 32 {
		in := (*[32]float32)(src[0:32])
		m0 = m0.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8])))
		m1 = m1.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16])))
		m2 = m2.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[16:24])))
		m3 = m3.Max(archsimd.LoadFloat32x8Array((*[8]float32)(in[24:32])))
		src = src[32:]
	}
	for len(src) >= 8 {
		m0 = m0.Max(archsimd.LoadFloat32x8Array((*[8]float32)(src[0:8])))
		src = src[8:]
	}
	if len(src) > 0 && n >= 8 {
		m1 = m1.Max(archsimd.LoadFloat32x8Array((*[8]float32)(all[n-8 : n])))
		src = nil
	}
	v8 := m0.Max(m1).Max(m2.Max(m3)).Max(m4.Max(m5).Max(m6.Max(m7)))
	mx := hmax4(v8.GetLo().Max(v8.GetHi()))
	for _, v := range src {
		mx = max(mx, v)
	}
	return mx
}

// Thirty-two 128-bit vectors (128 floats) per iteration over eight
// accumulators, each touched four times. 128 floats is 20-22% faster than the
// 64 arm64 uses from 16K up and no worse below.
//
// Reachable only via the X86VECTOR override: archsimd requires AVX even at
// 128 bits, and any CPU with AVX selects a wider path.
func vmaxF32x4(src []float32) float32 {
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x4(float32(math.Inf(-1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 128 {
		in := (*[128]float32)(src[0:128])
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
		m0 = m0.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[64:68])))
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[68:72])))
		m2 = m2.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[72:76])))
		m3 = m3.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[76:80])))
		m4 = m4.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[80:84])))
		m5 = m5.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[84:88])))
		m6 = m6.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[88:92])))
		m7 = m7.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[92:96])))
		m0 = m0.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[96:100])))
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[100:104])))
		m2 = m2.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[104:108])))
		m3 = m3.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[108:112])))
		m4 = m4.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[112:116])))
		m5 = m5.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[116:120])))
		m6 = m6.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[120:124])))
		m7 = m7.Max(archsimd.LoadFloat32x4Array((*[4]float32)(in[124:128])))
		src = src[128:]
	}
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
	if len(src) > 0 && n >= 4 {
		m1 = m1.Max(archsimd.LoadFloat32x4Array((*[4]float32)(all[n-4 : n])))
		src = nil
	}
	mx := hmax4(m0.Max(m1).Max(m2.Max(m3)).Max(m4.Max(m5).Max(m6.Max(m7))))
	for _, v := range src {
		mx = max(mx, v)
	}
	return mx
}

// Min returns the minimum value of src, or +Inf for an empty slice. NaN makes the result unspecified.
func Min(src []float32) float32 {
	if !haveSIMD() {
		// Pre-AVX CPU; see Scale.
		return vminF32Asm(src)
	}
	var mn float32
	switch {
	case simdWidth >= 512:
		mn = vminF32x16(src)
	case simdWidth >= 256:
		mn = vminF32x8(src)
	default:
		mn = vminF32x4(src)
	}
	archsimd.ClearAVXUpperBits() // see Scale
	return mn
}

// The min loops mirror the max ones above exactly: VMINPS costs the same, min
// is idempotent, and the reasoning is the same. It is written out once above.

// hmin4 returns the minimum of the four elements of v, staying in registers.
func hmin4(v archsimd.Float32x4) float32 {
	v = v.Min(v.ConcatPermuteScalars(2, 3, 2, 3, v))
	v = v.Min(v.ConcatPermuteScalars(1, 1, 1, 1, v))
	return v.GetElem(0)
}

// Eight 512-bit vectors (128 floats) per iteration, one accumulator each.
func vminF32x16(src []float32) float32 {
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x16(float32(math.Inf(1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 128 {
		in := (*[128]float32)(src[0:128])
		m0 = m0.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[0:16])))
		m1 = m1.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[16:32])))
		m2 = m2.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[32:48])))
		m3 = m3.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[48:64])))
		m4 = m4.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[64:80])))
		m5 = m5.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[80:96])))
		m6 = m6.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[96:112])))
		m7 = m7.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[112:128])))
		src = src[128:]
	}
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		m0 = m0.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[0:16])))
		m1 = m1.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[16:32])))
		m2 = m2.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[32:48])))
		m3 = m3.Min(archsimd.LoadFloat32x16Array((*[16]float32)(in[48:64])))
		src = src[64:]
	}
	for len(src) >= 16 {
		m0 = m0.Min(archsimd.LoadFloat32x16Array((*[16]float32)(src[0:16])))
		src = src[16:]
	}
	if len(src) > 0 && n >= 16 {
		m1 = m1.Min(archsimd.LoadFloat32x16Array((*[16]float32)(all[n-16 : n])))
		src = nil
	}
	v16 := m0.Min(m1).Min(m2.Min(m3)).Min(m4.Min(m5).Min(m6.Min(m7)))
	v8 := v16.GetLo().Min(v16.GetHi())
	mn := hmin4(v8.GetLo().Min(v8.GetHi()))
	for _, v := range src {
		mn = min(mn, v)
	}
	return mn
}

// Eight 256-bit vectors (64 floats) per iteration, one accumulator each.
func vminF32x8(src []float32) float32 {
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x8(float32(math.Inf(1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 64 {
		in := (*[64]float32)(src[0:64])
		m0 = m0.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8])))
		m1 = m1.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16])))
		m2 = m2.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[16:24])))
		m3 = m3.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[24:32])))
		m4 = m4.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[32:40])))
		m5 = m5.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[40:48])))
		m6 = m6.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[48:56])))
		m7 = m7.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[56:64])))
		src = src[64:]
	}
	for len(src) >= 32 {
		in := (*[32]float32)(src[0:32])
		m0 = m0.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8])))
		m1 = m1.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16])))
		m2 = m2.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[16:24])))
		m3 = m3.Min(archsimd.LoadFloat32x8Array((*[8]float32)(in[24:32])))
		src = src[32:]
	}
	for len(src) >= 8 {
		m0 = m0.Min(archsimd.LoadFloat32x8Array((*[8]float32)(src[0:8])))
		src = src[8:]
	}
	if len(src) > 0 && n >= 8 {
		m1 = m1.Min(archsimd.LoadFloat32x8Array((*[8]float32)(all[n-8 : n])))
		src = nil
	}
	v8 := m0.Min(m1).Min(m2.Min(m3)).Min(m4.Min(m5).Min(m6.Min(m7)))
	mn := hmin4(v8.GetLo().Min(v8.GetHi()))
	for _, v := range src {
		mn = min(mn, v)
	}
	return mn
}

// Thirty-two 128-bit vectors (128 floats) per iteration over eight
// accumulators, each touched four times. Reachable only via the X86VECTOR
// override; see vmaxF32x4.
func vminF32x4(src []float32) float32 {
	all, n := src, len(src)
	m0 := archsimd.BroadcastFloat32x4(float32(math.Inf(1)))
	m1, m2, m3, m4, m5, m6, m7 := m0, m0, m0, m0, m0, m0, m0
	for len(src) >= 128 {
		in := (*[128]float32)(src[0:128])
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
		m0 = m0.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[64:68])))
		m1 = m1.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[68:72])))
		m2 = m2.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[72:76])))
		m3 = m3.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[76:80])))
		m4 = m4.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[80:84])))
		m5 = m5.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[84:88])))
		m6 = m6.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[88:92])))
		m7 = m7.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[92:96])))
		m0 = m0.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[96:100])))
		m1 = m1.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[100:104])))
		m2 = m2.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[104:108])))
		m3 = m3.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[108:112])))
		m4 = m4.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[112:116])))
		m5 = m5.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[116:120])))
		m6 = m6.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[120:124])))
		m7 = m7.Min(archsimd.LoadFloat32x4Array((*[4]float32)(in[124:128])))
		src = src[128:]
	}
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
	mn := hmin4(m0.Min(m1).Min(m2.Min(m3)).Min(m4.Min(m5).Min(m6.Min(m7))))
	for _, v := range src {
		mn = min(mn, v)
	}
	return mn
}

// CAbs writes dst[i] = |src[i]| over min(len(src), len(dst)) elements.
func CAbs(dst []float32, src []complex64) {
	if !haveSIMD() {
		// Pre-AVX CPU; see Scale.
		vabsC64Asm(dst, src)
		return
	}
	n := min(len(src), len(dst))
	in, out := view.C64ToF32(src)[:n*2], dst[:n]
	switch {
	case simdWidth >= 512:
		vabsC64x16(out, in)
	case simdWidth >= 256:
		vabsC64x8(out, in)
	default:
		vabsC64x4(out, in)
	}
	archsimd.ClearAVXUpperBits() // see Scale
}

// The three loops below take the input already reinterpreted as float32 pairs,
// so in is exactly twice as long as out.
//
// These are square-root bound rather than bookkeeping bound — VSQRTPS is the
// only instruction here slower than one per cycle — and there is no accumulator
// chain to cover, so four output vectors per iteration is enough and wider
// mostly buys spills.
//
// What differs per width is deinterleaving the real and imaginary lanes:
// VPERMI2PS in one instruction at 512 bits; at 256 the loads are first
// restacked by half with VPERM2F128 (VSHUFPS selects only within a half and
// VPERMPD is AVX2); at 128, two VSHUFPS are the whole job.
//
// The squares are multiplied and added separately rather than with MulAdd; see
// vabsC64Scalar.

// absMag4 writes to out the magnitudes of the four complex values in in.
func absMag4(out *[4]float32, in *[8]float32) {
	a := archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4]))
	b := archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8]))
	re := a.ConcatPermuteScalars(0, 2, 4, 6, b)
	im := a.ConcatPermuteScalars(1, 3, 5, 7, b)
	re.Mul(re).Add(im.Mul(im)).Sqrt().StoreArray(out)
}

// absMag8 writes to out the magnitudes of the eight complex values in in.
func absMag8(out *[8]float32, in *[16]float32) {
	a := archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8]))
	b := archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16]))
	lo := a.ConcatPermute128Scalars(0, 2, b)
	hi := a.ConcatPermute128Scalars(1, 3, b)
	re := lo.ConcatPermuteScalarsGrouped(0, 2, 4, 6, hi)
	im := lo.ConcatPermuteScalarsGrouped(1, 3, 5, 7, hi)
	re.Mul(re).Add(im.Mul(im)).Sqrt().StoreArray(out)
}

// absMag16 writes to out the magnitudes of the sixteen complex values in in.
// ev and od are the lane indices built by vabsC64x16.
func absMag16(out *[16]float32, in *[32]float32, ev, od archsimd.Uint32x16) {
	a := archsimd.LoadFloat32x16Array((*[16]float32)(in[0:16]))
	b := archsimd.LoadFloat32x16Array((*[16]float32)(in[16:32]))
	re := a.ConcatPermute(b, ev)
	im := a.ConcatPermute(b, od)
	re.Mul(re).Add(im.Mul(im)).Sqrt().StoreArray(out)
}

// absEvenIdx and absOddIdx select the real and the imaginary lanes out of the
// concatenation of two 512-bit vectors of complex64.
var (
	absEvenIdx = [16]uint32{0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30}
	absOddIdx  = [16]uint32{1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 21, 23, 25, 27, 29, 31}
)

// Four 512-bit output vectors (64 magnitudes) per iteration.
//
// Each width finishes its remainder by recomputing the last full vector, which
// is harmless since |x| depends only on its own element. vabsC64Tail converts
// through float64 at ~28 cycles per element against 0.78 for the vector loop,
// so it is reserved for sub-vector inputs.
func vabsC64x16(out, in []float32) {
	allIn, allOut, n := in, out, len(out)
	ev := archsimd.LoadUint32x16Array(&absEvenIdx)
	od := archsimd.LoadUint32x16Array(&absOddIdx)
	for len(out) >= 64 {
		i := (*[128]float32)(in[0:128])
		o := (*[64]float32)(out[0:64])
		absMag16((*[16]float32)(o[0:16]), (*[32]float32)(i[0:32]), ev, od)
		absMag16((*[16]float32)(o[16:32]), (*[32]float32)(i[32:64]), ev, od)
		absMag16((*[16]float32)(o[32:48]), (*[32]float32)(i[64:96]), ev, od)
		absMag16((*[16]float32)(o[48:64]), (*[32]float32)(i[96:128]), ev, od)
		in, out = in[128:], out[64:]
	}
	for len(out) >= 16 {
		absMag16((*[16]float32)(out[0:16]), (*[32]float32)(in[0:32]), ev, od)
		in, out = in[32:], out[16:]
	}
	if len(out) > 0 && n >= 16 {
		absMag16((*[16]float32)(allOut[n-16:n]), (*[32]float32)(allIn[(n-16)*2:n*2]), ev, od)
		return
	}
	vabsC64Tail(out, in)
}

// Four 256-bit output vectors (32 magnitudes) per iteration.
func vabsC64x8(out, in []float32) {
	allIn, allOut, n := in, out, len(out)
	for len(out) >= 32 {
		i := (*[64]float32)(in[0:64])
		o := (*[32]float32)(out[0:32])
		absMag8((*[8]float32)(o[0:8]), (*[16]float32)(i[0:16]))
		absMag8((*[8]float32)(o[8:16]), (*[16]float32)(i[16:32]))
		absMag8((*[8]float32)(o[16:24]), (*[16]float32)(i[32:48]))
		absMag8((*[8]float32)(o[24:32]), (*[16]float32)(i[48:64]))
		in, out = in[64:], out[32:]
	}
	for len(out) >= 8 {
		absMag8((*[8]float32)(out[0:8]), (*[16]float32)(in[0:16]))
		in, out = in[16:], out[8:]
	}
	if len(out) > 0 && n >= 8 {
		absMag8((*[8]float32)(allOut[n-8:n]), (*[16]float32)(allIn[(n-8)*2:n*2]))
		return
	}
	vabsC64Tail(out, in)
}

// Four 128-bit output vectors (16 magnitudes) per iteration. Reachable only
// via the X86VECTOR override; see vmaxF32x4.
func vabsC64x4(out, in []float32) {
	allIn, allOut, n := in, out, len(out)
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
	if len(out) > 0 && n >= 4 {
		absMag4((*[4]float32)(allOut[n-4:n]), (*[8]float32)(allIn[(n-4)*2:n*2]))
		return
	}
	vabsC64Tail(out, in)
}

// vabsC64Tail finishes the remainder shorter than one vector.
func vabsC64Tail(out, in []float32) {
	for i := range out {
		re, im := in[i*2], in[i*2+1]
		out[i] = float32(math.Sqrt(float64(float32(re*re) + float32(im*im))))
	}
}
