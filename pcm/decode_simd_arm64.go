//go:build goexperiment.simd

package pcm

import (
	"simd/archsimd"
	"unsafe"
)

// The U8, I8 and I16LE kernels into float32 are archsimd below; the rest are
// the references.
//
// Each converts to float32 and multiplies by 2^-(bits-1). The integers are
// exact in a float32 and the scale is a power of two, so the product is exact
// and matches the reference bit for bit. The remainder shorter than a vector
// runs the reference.
//
// archsimd has no fixed-point convert, which the assembly uses to fold the
// scale into the convert, so the multiply costs these about a fifth against
// an unscaled loop. Building the float from its bits instead (the code ORed
// into the mantissa of a power of two, then a subtract) measured no better.
//
// Loads and stores go through *[N]T rather than the slice forms, which emit a
// bounds check and re-derive the element pointer on every access. arm64 has no
// single-step byte-to-int32 extension, so the bytes widen through int16.

func u8ToF32(dst []float32, src []byte) {
	n := min(len(src), len(dst))
	bias := archsimd.BroadcastUint8x16(128)
	scale := archsimd.BroadcastFloat32x4(1.0 / (1 << 7))
	i := 0
	for ; i+64 <= n; i += 64 {
		in := (*[64]byte)(src[i : i+64])
		out := (*[64]float32)(dst[i : i+64])
		bytesToF32((*[16]float32)(out[0:16]), archsimd.LoadUint8x16Array((*[16]uint8)(in[0:16])).Sub(bias).BitsToInt8(), scale)
		bytesToF32((*[16]float32)(out[16:32]), archsimd.LoadUint8x16Array((*[16]uint8)(in[16:32])).Sub(bias).BitsToInt8(), scale)
		bytesToF32((*[16]float32)(out[32:48]), archsimd.LoadUint8x16Array((*[16]uint8)(in[32:48])).Sub(bias).BitsToInt8(), scale)
		bytesToF32((*[16]float32)(out[48:64]), archsimd.LoadUint8x16Array((*[16]uint8)(in[48:64])).Sub(bias).BitsToInt8(), scale)
	}
	for ; i+16 <= n; i += 16 {
		bytesToF32((*[16]float32)(dst[i:i+16]), archsimd.LoadUint8x16Array((*[16]uint8)(src[i:i+16])).Sub(bias).BitsToInt8(), scale)
	}
	u8ToFloatRef(dst[i:n], src[i:n])
}

func i8ToF32(dst []float32, src []byte) {
	n := min(len(src), len(dst))
	scale := archsimd.BroadcastFloat32x4(1.0 / (1 << 7))
	i := 0
	for ; i+64 <= n; i += 64 {
		in := (*[64]byte)(src[i : i+64])
		out := (*[64]float32)(dst[i : i+64])
		bytesToF32((*[16]float32)(out[0:16]), archsimd.LoadUint8x16Array((*[16]uint8)(in[0:16])).BitsToInt8(), scale)
		bytesToF32((*[16]float32)(out[16:32]), archsimd.LoadUint8x16Array((*[16]uint8)(in[16:32])).BitsToInt8(), scale)
		bytesToF32((*[16]float32)(out[32:48]), archsimd.LoadUint8x16Array((*[16]uint8)(in[32:48])).BitsToInt8(), scale)
		bytesToF32((*[16]float32)(out[48:64]), archsimd.LoadUint8x16Array((*[16]uint8)(in[48:64])).BitsToInt8(), scale)
	}
	for ; i+16 <= n; i += 16 {
		bytesToF32((*[16]float32)(dst[i:i+16]), archsimd.LoadUint8x16Array((*[16]uint8)(src[i:i+16])).BitsToInt8(), scale)
	}
	i8ToFloatRef(dst[i:n], src[i:n])
}

// bytesToF32 writes sixteen signed bytes, scaled, to out.
func bytesToF32(out *[16]float32, b archsimd.Int8x16, scale archsimd.Float32x4) {
	lo := b.ExtendLo8ToInt16()
	hi := b.HiToLo().ExtendLo8ToInt16()
	lo.ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[0:4]))
	lo.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[4:8]))
	hi.ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[8:12]))
	hi.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[12:16]))
}

// i16LEToF32 reads the bytes as int16 in place: arm64 is little-endian and
// allows unaligned vector loads.
func i16LEToF32(dst []float32, src []byte) {
	n := min(len(src)/2, len(dst))
	scale := archsimd.BroadcastFloat32x4(1.0 / (1 << 15))
	i := 0
	for ; i+16 <= n; i += 16 {
		in := src[2*i : 2*i+32]
		out := (*[16]float32)(dst[i : i+16])
		a := archsimd.LoadInt16x8Array((*[8]int16)(unsafe.Pointer(&in[0])))
		b := archsimd.LoadInt16x8Array((*[8]int16)(unsafe.Pointer(&in[16])))
		a.ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[0:4]))
		a.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[4:8]))
		b.ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[8:12]))
		b.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().Mul(scale).StoreArray((*[4]float32)(out[12:16]))
	}
	i16LEToFloatRef(dst[i:n], src[2*i:2*n])
}

func u8ToF64(dst []float64, src []byte)    { u8ToFloatRef(dst, src) }
func i8ToF64(dst []float64, src []byte)    { i8ToFloatRef(dst, src) }
func i16LEToF64(dst []float64, src []byte) { i16LEToFloatRef(dst, src) }
func i24LEToF32(dst []float32, src []byte) { i24LEToFloatRef(dst, src) }
func i24LEToF64(dst []float64, src []byte) { i24LEToFloatRef(dst, src) }
func i32LEToF32(dst []float32, src []byte) { i32LEToFloatRef(dst, src) }
func i32LEToF64(dst []float64, src []byte) { i32LEToFloatRef(dst, src) }
func f32LEToF32(dst []float32, src []byte) { f32LEToFloatRef(dst, src) }
func f32LEToF64(dst []float64, src []byte) { f32LEToFloatRef(dst, src) }
func f64LEToF32(dst []float32, src []byte) { f64LEToFloatRef(dst, src) }
func f64LEToF64(dst []float64, src []byte) { f64LEToFloatRef(dst, src) }
func alawToF32(dst []float32, src []byte)  { alawToFloatRef(dst, src) }
func alawToF64(dst []float64, src []byte)  { alawToFloatRef(dst, src) }
func mulawToF32(dst []float32, src []byte) { mulawToFloatRef(dst, src) }
func mulawToF64(dst []float64, src []byte) { mulawToFloatRef(dst, src) }
