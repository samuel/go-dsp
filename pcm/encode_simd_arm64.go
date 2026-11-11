//go:build goexperiment.simd

package pcm

import (
	"simd/archsimd"
	"unsafe"
)

// The float32 to I16LE kernel is archsimd below; the rest are the references.

// f32ToI16LE multiplies by 2^15, which is exact or overflows to an infinity,
// rounds to nearest even (FRINTN), and converts the now integral value
// (FCVTZS, which saturates at the int32 range and turns NaN into zero) before
// saturating to int16 (SQXTN). Saturating after rounding clips at the same
// codes as the reference, which clips first: the rails are integers.
//
// SQXTN fills only the low half of a vector, so four of them are gathered
// into two with a zip and an unzip. The int16 results are stored into the
// bytes in place: arm64 is little-endian and allows unaligned stores. The
// remainder shorter than sixteen samples runs the reference.
func f32ToI16LE(dst []byte, src []float32) {
	n := min(len(dst)/2, len(src))
	full := archsimd.BroadcastFloat32x4(1 << 15)
	i := 0
	for ; i+16 <= n; i += 16 {
		in := (*[16]float32)(src[i : i+16])
		out := dst[2*i : 2*i+32]
		a := toI16(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])), full)
		b := toI16(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])), full)
		c := toI16(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])), full)
		d := toI16(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])), full)
		// p = a0 c0 a1 c1 ..., q = b0 d0 b1 d1 ...; the even lanes of the
		// pair are a then b, the odd ones c then d.
		p, q := a.InterleaveLo(c), b.InterleaveLo(d)
		p.ConcatEven(q).StoreArray((*[8]int16)(unsafe.Pointer(&out[0])))
		p.ConcatOdd(q).StoreArray((*[8]int16)(unsafe.Pointer(&out[16])))
	}
	floatToI16LERef(dst[2*i:2*n], src[i:n])
}

// toI16 returns four samples as int16 codes in the low half of a vector.
func toI16(v, full archsimd.Float32x4) archsimd.Int16x8 {
	return v.Mul(full).Round().ConvertToInt32().SaturateToInt16()
}

func f32ToU8(dst []byte, src []float32)    { floatToU8Ref(dst, src) }
func f64ToU8(dst []byte, src []float64)    { floatToU8Ref(dst, src) }
func f32ToI8(dst []byte, src []float32)    { floatToI8Ref(dst, src) }
func f64ToI8(dst []byte, src []float64)    { floatToI8Ref(dst, src) }
func f64ToI16LE(dst []byte, src []float64) { floatToI16LERef(dst, src) }
func f32ToI24LE(dst []byte, src []float32) { floatToI24LERef(dst, src) }
func f64ToI24LE(dst []byte, src []float64) { floatToI24LERef(dst, src) }
func f32ToI32LE(dst []byte, src []float32) { floatToI32LERef(dst, src) }
func f64ToI32LE(dst []byte, src []float64) { floatToI32LERef(dst, src) }
func f32ToF32LE(dst []byte, src []float32) { floatToF32LERef(dst, src) }
func f64ToF32LE(dst []byte, src []float64) { floatToF32LERef(dst, src) }
func f32ToF64LE(dst []byte, src []float32) { floatToF64LERef(dst, src) }
func f64ToF64LE(dst []byte, src []float64) { floatToF64LERef(dst, src) }
