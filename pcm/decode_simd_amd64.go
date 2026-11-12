//go:build goexperiment.simd

package pcm

import "simd/archsimd"

// Under GOEXPERIMENT=simd the vector kernels are archsimd, at the width
// simdWidth allows, falling back to the generated assembly on a CPU without
// AVX. i16LEToF64 is the assembly at every width.
func u8ToF32(dst []float32, src []byte)    { bytesToF32(dst, src, 128) }
func u8ToF64(dst []float64, src []byte)    { u8ToFloatRef(dst, src) }
func i8ToF32(dst []float32, src []byte)    { bytesToF32(dst, src, 0) }
func i8ToF64(dst []float64, src []byte)    { i8ToFloatRef(dst, src) }
func i16LEToF32(dst []float32, src []byte) { i16ToF32SIMD(dst, src) }
func i16LEToF64(dst []float64, src []byte) { i16LEToF64Asm(dst, src) }
func i24LEToF32(dst []float32, src []byte) { i24LEToFloatRef(dst, src) }
func i24LEToF64(dst []float64, src []byte) { i24LEToFloatRef(dst, src) }
func i32LEToF32(dst []float32, src []byte) { i32LEToFloatRef(dst, src) }
func i32LEToF64(dst []float64, src []byte) { i32LEToFloatRef(dst, src) }
func f32LEToF32(dst []float32, src []byte) { f32LEToFloatRef(dst, src) }
func f32LEToF64(dst []float64, src []byte) { f32LEToFloatRef(dst, src) }
func f64LEToF32(dst []float32, src []byte) { f64LEToFloatRef(dst, src) }
func f64LEToF64(dst []float64, src []byte) { f64LEToFloatRef(dst, src) }

// useAVX2 gates the 256-bit integer widening, VPMOVSXBD and VPMOVSXWD on ymm,
// which simdWidth's 256 does not promise: it needs only AVX. Without AVX2 the
// 256-bit width runs the 128-bit kernels.
var useAVX2 = archsimd.X86.AVX2()

// bytesToF32 decodes U8 (bias 128) or I8 (bias 0). Subtracting the bias as a
// wrapping byte leaves the signed sample, so both are one kernel.
func bytesToF32(dst []float32, src []byte, bias uint8) {
	if !haveSIMD() {
		// Pre-AVX CPU: archsimd is VEX-encoded even at 128 bits.
		if bias != 0 {
			u8ToF32Asm(dst, src)
		} else {
			i8ToF32Asm(dst, src)
		}
		return
	}
	n := min(len(src), len(dst))
	switch {
	case simdWidth >= 512:
		bytesToF32x16(dst[:n], src[:n], bias)
	case simdWidth >= 256 && useAVX2:
		bytesToF32x8(dst[:n], src[:n], bias)
	default:
		bytesToF32x4(dst[:n], src[:n], bias)
	}
	archsimd.ClearAVXUpperBits()
}

// Each width converts sixteen samples per block. When the length is not a
// multiple of the block it recomputes the last one, harmless since each output
// depends only on its own input; shorter inputs run the scalar tail. The
// blocks are named functions rather than closures so that they inline.

func bytesToF32x4(dst []float32, src []byte, bias uint8) {
	b, s := archsimd.BroadcastUint8x16(bias), archsimd.BroadcastFloat32x4(1.0/128)
	n, i := len(dst), 0
	for ; i+16 <= n; i += 16 {
		bytesBlockX4((*[16]float32)(dst[i:i+16]), (*[16]byte)(src[i:i+16]), b, s)
	}
	if i < n && n >= 16 {
		bytesBlockX4((*[16]float32)(dst[n-16:n]), (*[16]byte)(src[n-16:n]), b, s)
		return
	}
	bytesTail(dst[i:], src[i:], bias)
}

func bytesBlockX4(out *[16]float32, in *[16]byte, b archsimd.Uint8x16, s archsimd.Float32x4) {
	u := archsimd.LoadUint8x16Array(in).Sub(b)
	u.BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[4]float32)(out[0:4]))
	u.ConcatShiftBytesRight(u, 4).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[4]float32)(out[4:8]))
	u.ConcatShiftBytesRight(u, 8).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[4]float32)(out[8:12]))
	u.ConcatShiftBytesRight(u, 12).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[4]float32)(out[12:16]))
}

func bytesToF32x8(dst []float32, src []byte, bias uint8) {
	b, s := archsimd.BroadcastUint8x16(bias), archsimd.BroadcastFloat32x8(1.0/128)
	n, i := len(dst), 0
	for ; i+16 <= n; i += 16 {
		bytesBlockX8((*[16]float32)(dst[i:i+16]), (*[16]byte)(src[i:i+16]), b, s)
	}
	if i < n && n >= 16 {
		bytesBlockX8((*[16]float32)(dst[n-16:n]), (*[16]byte)(src[n-16:n]), b, s)
		return
	}
	bytesTail(dst[i:], src[i:], bias)
}

func bytesBlockX8(out *[16]float32, in *[16]byte, b archsimd.Uint8x16, s archsimd.Float32x8) {
	u := archsimd.LoadUint8x16Array(in).Sub(b)
	u.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[8]float32)(out[0:8]))
	u.ConcatShiftBytesRight(u, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[8]float32)(out[8:16]))
}

func bytesToF32x16(dst []float32, src []byte, bias uint8) {
	b, s := archsimd.BroadcastUint8x16(bias), archsimd.BroadcastFloat32x16(1.0/128)
	n, i := len(dst), 0
	for ; i+16 <= n; i += 16 {
		archsimd.LoadUint8x16Array((*[16]byte)(src[i : i+16])).Sub(b).BitsToInt8().ExtendToInt32().
			ConvertToFloat32().Mul(s).StoreArray((*[16]float32)(dst[i : i+16]))
	}
	if i < n && n >= 16 {
		archsimd.LoadUint8x16Array((*[16]byte)(src[n-16 : n])).Sub(b).BitsToInt8().ExtendToInt32().
			ConvertToFloat32().Mul(s).StoreArray((*[16]float32)(dst[n-16 : n]))
		return
	}
	bytesTail(dst[i:], src[i:], bias)
}

func bytesTail(dst []float32, src []byte, bias uint8) {
	for i, v := range src {
		dst[i] = float32(int8(v-bias)) * (1.0 / (1 << 7))
	}
}

func i16ToF32SIMD(dst []float32, src []byte) {
	if !haveSIMD() {
		i16LEToF32Asm(dst, src)
		return
	}
	n := min(len(dst), len(src)/2)
	switch {
	case simdWidth >= 512:
		i16ToF32x16(dst[:n], src[:2*n])
	case simdWidth >= 256 && useAVX2:
		i16ToF32x8(dst[:n], src[:2*n])
	default:
		i16ToF32x4(dst[:n], src[:2*n])
	}
	archsimd.ClearAVXUpperBits()
}

// The int16 kernels convert eight samples, sixteen bytes, per block, or
// sixteen at 512 bits, with the same overlapping last block.

func i16ToF32x4(dst []float32, src []byte) {
	s := archsimd.BroadcastFloat32x4(1.0 / (1 << 15))
	n, i := len(dst), 0
	for ; i+8 <= n; i += 8 {
		i16BlockX4((*[8]float32)(dst[i:i+8]), (*[16]byte)(src[2*i:2*i+16]), s)
	}
	if i < n && n >= 8 {
		i16BlockX4((*[8]float32)(dst[n-8:n]), (*[16]byte)(src[2*n-16:2*n]), s)
		return
	}
	i16LEToFloatRef(dst[i:], src[2*i:])
}

func i16BlockX4(out *[8]float32, in *[16]byte, s archsimd.Float32x4) {
	u := archsimd.LoadUint8x16Array(in)
	u.ReshapeToUint16s().BitsToInt16().ExtendLo4ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[4]float32)(out[0:4]))
	u.ConcatShiftBytesRight(u, 8).ReshapeToUint16s().BitsToInt16().ExtendLo4ToInt32().ConvertToFloat32().Mul(s).StoreArray((*[4]float32)(out[4:8]))
}

func i16ToF32x8(dst []float32, src []byte) {
	s := archsimd.BroadcastFloat32x8(1.0 / (1 << 15))
	n, i := len(dst), 0
	for ; i+8 <= n; i += 8 {
		archsimd.LoadUint8x16Array((*[16]byte)(src[2*i : 2*i+16])).ReshapeToUint16s().BitsToInt16().ExtendToInt32().
			ConvertToFloat32().Mul(s).StoreArray((*[8]float32)(dst[i : i+8]))
	}
	if i < n && n >= 8 {
		archsimd.LoadUint8x16Array((*[16]byte)(src[2*n-16 : 2*n])).ReshapeToUint16s().BitsToInt16().ExtendToInt32().
			ConvertToFloat32().Mul(s).StoreArray((*[8]float32)(dst[n-8 : n]))
		return
	}
	i16LEToFloatRef(dst[i:], src[2*i:])
}

func i16ToF32x16(dst []float32, src []byte) {
	s := archsimd.BroadcastFloat32x16(1.0 / (1 << 15))
	n, i := len(dst), 0
	for ; i+16 <= n; i += 16 {
		archsimd.LoadUint8x32Array((*[32]byte)(src[2*i : 2*i+32])).ReshapeToUint16s().BitsToInt16().ExtendToInt32().
			ConvertToFloat32().Mul(s).StoreArray((*[16]float32)(dst[i : i+16]))
	}
	if i < n && n >= 16 {
		archsimd.LoadUint8x32Array((*[32]byte)(src[2*n-32 : 2*n])).ReshapeToUint16s().BitsToInt16().ExtendToInt32().
			ConvertToFloat32().Mul(s).StoreArray((*[16]float32)(dst[n-16 : n]))
		return
	}
	i16LEToFloatRef(dst[i:], src[2*i:])
}
func alawToF32(dst []float32, src []byte)  { alawToFloatRef(dst, src) }
func alawToF64(dst []float64, src []byte)  { alawToFloatRef(dst, src) }
func mulawToF32(dst []float32, src []byte) { mulawToFloatRef(dst, src) }
func mulawToF64(dst []float64, src []byte) { mulawToFloatRef(dst, src) }
