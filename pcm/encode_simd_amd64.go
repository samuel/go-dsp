//go:build goexperiment.simd

package pcm

import "simd/archsimd"

// Under GOEXPERIMENT=simd the vector kernel is archsimd, falling back to the
// generated assembly on a CPU without AVX.
func f32ToU8(dst []byte, src []float32)    { floatToU8Ref(dst, src) }
func f64ToU8(dst []byte, src []float64)    { floatToU8Ref(dst, src) }
func f32ToI8(dst []byte, src []float32)    { floatToI8Ref(dst, src) }
func f64ToI8(dst []byte, src []float64)    { floatToI8Ref(dst, src) }
func f32ToI16LE(dst []byte, src []float32) { f32ToI16SIMD(dst, src) }
func f64ToI16LE(dst []byte, src []float64) { floatToI16LERef(dst, src) }
func f32ToI24LE(dst []byte, src []float32) { floatToI24LERef(dst, src) }
func f64ToI24LE(dst []byte, src []float64) { floatToI24LERef(dst, src) }
func f32ToI32LE(dst []byte, src []float32) { floatToI32LERef(dst, src) }
func f64ToI32LE(dst []byte, src []float64) { floatToI32LERef(dst, src) }
func f32ToF32LE(dst []byte, src []float32) { floatToF32LERef(dst, src) }
func f64ToF32LE(dst []byte, src []float64) { floatToF32LERef(dst, src) }
func f32ToF64LE(dst []byte, src []float32) { floatToF64LERef(dst, src) }
func f64ToF64LE(dst []byte, src []float64) { floatToF64LERef(dst, src) }

// f32ToI16SIMD multiplies by 32768, zeroes NaN, clips to the int16 rails,
// rounds to nearest even with VROUNDPS, and packs. The convert after rounding
// is exact, so its truncation does not matter. 512-bit CPUs run the 256-bit
// kernel.
func f32ToI16SIMD(dst []byte, src []float32) {
	if !haveSIMD() {
		f32ToI16LEAsm(dst, src)
		return
	}
	n := min(len(dst)/2, len(src))
	if simdWidth >= 256 && useAVX2 {
		f32ToI16x8(dst[:2*n], src[:n])
	} else {
		f32ToI16x4(dst[:2*n], src[:n])
	}
	archsimd.ClearAVXUpperBits()
}

// Both widths encode eight samples per block, recomputing the last block when
// the length is not a multiple of eight; shorter inputs run the reference.

func f32ToI16x4(dst []byte, src []float32) {
	m := archsimd.BroadcastFloat32x4(1 << 15)
	lo := archsimd.BroadcastFloat32x4(-(1 << 15))
	hi := archsimd.BroadcastFloat32x4(1<<15 - 1)
	n, i := len(src), 0
	for ; i+8 <= n; i += 8 {
		f32ToI16BlockX4((*[16]byte)(dst[2*i:2*i+16]), (*[8]float32)(src[i:i+8]), m, lo, hi)
	}
	if i < n && n >= 8 {
		f32ToI16BlockX4((*[16]byte)(dst[2*n-16:2*n]), (*[8]float32)(src[n-8:n]), m, lo, hi)
		return
	}
	floatToI16LERef(dst[2*i:], src[i:])
}

func f32ToI16BlockX4(out *[16]byte, in *[8]float32, m, lo, hi archsimd.Float32x4) {
	a := archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).Mul(m)
	b := archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).Mul(m)
	a = a.Masked(a.Equal(a)).Max(lo).Min(hi).Round() //nolint:gocritic // Equal is false only in NaN lanes
	b = b.Masked(b.Equal(b)).Max(lo).Min(hi).Round() //nolint:gocritic // Equal is false only in NaN lanes
	a.ConvertToInt32().SaturateToInt16Concat(b.ConvertToInt32()).ToBits().ReshapeToUint8s().StoreArray(out)
}

// f32ToI16x8 needs AVX2 for the 256-bit integer AND behind Masked and for
// splitting the int32 halves.
func f32ToI16x8(dst []byte, src []float32) {
	m := archsimd.BroadcastFloat32x8(1 << 15)
	lo := archsimd.BroadcastFloat32x8(-(1 << 15))
	hi := archsimd.BroadcastFloat32x8(1<<15 - 1)
	n, i := len(src), 0
	for ; i+8 <= n; i += 8 {
		f32ToI16BlockX8((*[16]byte)(dst[2*i:2*i+16]), (*[8]float32)(src[i:i+8]), m, lo, hi)
	}
	if i < n && n >= 8 {
		f32ToI16BlockX8((*[16]byte)(dst[2*n-16:2*n]), (*[8]float32)(src[n-8:n]), m, lo, hi)
		return
	}
	floatToI16LERef(dst[2*i:], src[i:])
}

func f32ToI16BlockX8(out *[16]byte, in *[8]float32, m, lo, hi archsimd.Float32x8) {
	v := archsimd.LoadFloat32x8Array(in).Mul(m)
	v = v.Masked(v.Equal(v)).Max(lo).Min(hi).Round() //nolint:gocritic // Equal is false only in NaN lanes
	q := v.ConvertToInt32()
	q.GetLo().SaturateToInt16Concat(q.GetHi()).ToBits().ReshapeToUint8s().StoreArray(out)
}
