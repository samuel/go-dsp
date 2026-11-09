//go:build goexperiment.simd

package encoding

import "simd/archsimd"

// U8ToF32 converts unsigned 8-bit samples to float32, subtracting the 128 bias.
// It does not scale: the range is -128 to 127.
func U8ToF32(dst []float32, src []byte) {
	if !haveSIMD() {
		// Pre-AVX CPU; see f32.Scale. The generated assembly covers every
		// amd64 CPU, including the ones archsimd cannot target at all.
		u8ToF32Asm(dst, src)
		return
	}
	n := min(len(src), len(dst))
	switch {
	case simdWidth >= 512:
		u8ToF32x16(dst[:n], src[:n])
	case simdWidth >= 256:
		u8ToF32x8(dst[:n], src[:n])
	default:
		u8ToF32x4(dst[:n], src[:n])
	}
	archsimd.ClearAVXUpperBits() // see f32.Scale
}

// All three widths strip the 128 bias with one wrapping byte subtract before
// widening: for a uint8 sample s, the byte s-128 as int8 is exactly s-128, so
// one PSUBB across sixteen lanes replaces sixteen scalar subtractions and the
// widening is a plain sign extension. Results are small integers, so the
// float32 conversion is exact and all three agree bit for bit with the scalar
// loop.
//
// VPMOVSXBD sign-extends bytes straight to int32, so there is no int16 step as
// on arm64; the widths differ only in how the rest of a sixteen-byte load is
// brought to the low lanes (VPALIGNR by 4 or 8 bytes). The 512-bit path needs
// none of that and is faster even after the frequency license, though
// avx512Downclocks steers it to 256 anyway.
//
// Block sizes were swept on a Xeon Silver 4210: 128 samples per iteration for
// the 256-bit path, 64 for the flatter 512-bit one.
//
// Each width finishes its remainder by recomputing the last sixteen samples
// rather than u8ToF32Tail, harmless since each output depends only on its own
// input byte. That mattered once the main loop got wider: a fifteen-sample
// remainder cost 41% on top of a 1024-sample call.

// Four samples per extension, sixteen per load. 32 samples per iteration.
//
// Reachable only through the X86VECTOR override, for the reason given on
// vmaxF32x4.
func u8ToF32x4(dst []float32, src []byte) {
	allIn, allOut, n := src, dst, len(dst)
	bias := archsimd.BroadcastUint8x16(128)
	for len(dst) >= 16 {
		b := archsimd.LoadUint8x16Array((*[16]uint8)(src[0:16])).Sub(bias)
		out := (*[16]float32)(dst[0:16])
		b.BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[0:4]))
		b.ConcatShiftBytesRight(b, 4).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[4:8]))
		b.ConcatShiftBytesRight(b, 8).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[8:12]))
		b.ConcatShiftBytesRight(b, 12).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[12:16]))
		src, dst = src[16:], dst[16:]
	}
	if len(dst) > 0 && n >= 16 {
		b := archsimd.LoadUint8x16Array((*[16]uint8)(allIn[n-16 : n])).Sub(bias)
		out := (*[16]float32)(allOut[n-16 : n])
		b.BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[0:4]))
		b.ConcatShiftBytesRight(b, 4).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[4:8]))
		b.ConcatShiftBytesRight(b, 8).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[8:12]))
		b.ConcatShiftBytesRight(b, 12).BitsToInt8().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[12:16]))
		return
	}
	u8ToF32Tail(dst, src)
}

// Eight samples per extension, sixteen per load. This is the width most
// hardware selects.
func u8ToF32x8(dst []float32, src []byte) {
	allIn, allOut, n := src, dst, len(dst)
	bias := archsimd.BroadcastUint8x16(128)
	for len(dst) >= 128 {
		in := (*[128]byte)(src[0:128])
		out := (*[128]float32)(dst[0:128])
		b0 := archsimd.LoadUint8x16Array((*[16]uint8)(in[0:16])).Sub(bias)
		b1 := archsimd.LoadUint8x16Array((*[16]uint8)(in[16:32])).Sub(bias)
		b2 := archsimd.LoadUint8x16Array((*[16]uint8)(in[32:48])).Sub(bias)
		b3 := archsimd.LoadUint8x16Array((*[16]uint8)(in[48:64])).Sub(bias)
		b4 := archsimd.LoadUint8x16Array((*[16]uint8)(in[64:80])).Sub(bias)
		b5 := archsimd.LoadUint8x16Array((*[16]uint8)(in[80:96])).Sub(bias)
		b6 := archsimd.LoadUint8x16Array((*[16]uint8)(in[96:112])).Sub(bias)
		b7 := archsimd.LoadUint8x16Array((*[16]uint8)(in[112:128])).Sub(bias)
		b0.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[0:8]))
		b0.ConcatShiftBytesRight(b0, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[8:16]))
		b1.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[16:24]))
		b1.ConcatShiftBytesRight(b1, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[24:32]))
		b2.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[32:40]))
		b2.ConcatShiftBytesRight(b2, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[40:48]))
		b3.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[48:56]))
		b3.ConcatShiftBytesRight(b3, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[56:64]))
		b4.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[64:72]))
		b4.ConcatShiftBytesRight(b4, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[72:80]))
		b5.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[80:88]))
		b5.ConcatShiftBytesRight(b5, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[88:96]))
		b6.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[96:104]))
		b6.ConcatShiftBytesRight(b6, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[104:112]))
		b7.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[112:120]))
		b7.ConcatShiftBytesRight(b7, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[120:128]))
		src, dst = src[128:], dst[128:]
	}
	for len(dst) >= 16 {
		b := archsimd.LoadUint8x16Array((*[16]uint8)(src[0:16])).Sub(bias)
		out := (*[16]float32)(dst[0:16])
		b.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[0:8]))
		b.ConcatShiftBytesRight(b, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[8:16]))
		src, dst = src[16:], dst[16:]
	}
	if len(dst) > 0 && n >= 16 {
		b := archsimd.LoadUint8x16Array((*[16]uint8)(allIn[n-16 : n])).Sub(bias)
		out := (*[16]float32)(allOut[n-16 : n])
		b.BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[0:8]))
		b.ConcatShiftBytesRight(b, 8).BitsToInt8().ExtendLo8ToInt32().ConvertToFloat32().StoreArray((*[8]float32)(out[8:16]))
		return
	}
	u8ToF32Tail(dst, src)
}

func u8ToF32x16(dst []float32, src []byte) {
	allIn, allOut, n := src, dst, len(dst)
	bias := archsimd.BroadcastUint8x16(128)
	for len(dst) >= 64 {
		in := (*[64]byte)(src[0:64])
		out := (*[64]float32)(dst[0:64])
		b0 := archsimd.LoadUint8x16Array((*[16]uint8)(in[0:16])).Sub(bias)
		b1 := archsimd.LoadUint8x16Array((*[16]uint8)(in[16:32])).Sub(bias)
		b2 := archsimd.LoadUint8x16Array((*[16]uint8)(in[32:48])).Sub(bias)
		b3 := archsimd.LoadUint8x16Array((*[16]uint8)(in[48:64])).Sub(bias)
		b0.BitsToInt8().ExtendToInt32().ConvertToFloat32().StoreArray((*[16]float32)(out[0:16]))
		b1.BitsToInt8().ExtendToInt32().ConvertToFloat32().StoreArray((*[16]float32)(out[16:32]))
		b2.BitsToInt8().ExtendToInt32().ConvertToFloat32().StoreArray((*[16]float32)(out[32:48]))
		b3.BitsToInt8().ExtendToInt32().ConvertToFloat32().StoreArray((*[16]float32)(out[48:64]))
		src, dst = src[64:], dst[64:]
	}
	for len(dst) >= 16 {
		archsimd.LoadUint8x16Array((*[16]uint8)(src[0:16])).Sub(bias).
			BitsToInt8().ExtendToInt32().ConvertToFloat32().StoreArray((*[16]float32)(dst[0:16]))
		src, dst = src[16:], dst[16:]
	}
	if len(dst) > 0 && n >= 16 {
		archsimd.LoadUint8x16Array((*[16]uint8)(allIn[n-16 : n])).Sub(bias).
			BitsToInt8().ExtendToInt32().ConvertToFloat32().StoreArray((*[16]float32)(allOut[n-16 : n]))
		return
	}
	u8ToF32Tail(dst, src)
}

// u8ToF32Tail finishes the remainder shorter than one sixteen-byte load.
func u8ToF32Tail(dst []float32, src []byte) {
	for i, v := range src {
		dst[i] = float32(int(v) - 128)
	}
}
