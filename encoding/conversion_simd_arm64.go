//go:build goexperiment.simd

package encoding

import "simd/archsimd"

// U8ToF32 converts unsigned 8-bit samples to float32, subtracting the 128 bias.
// It does not scale: the range is -128 to 127.
//
// The bias comes off with one wrapping byte subtract before any widening: the
// byte s-128 as int8 is exactly s-128, so one SUB across sixteen lanes replaces
// sixteen scalar subtractions, and each result is a small integer so the
// float32 conversion is exact and matches the scalar loop bit for bit.
//
// arm64 has no single-step byte-to-int32 extension, so the widening goes
// through int16 (the HiToLo pairs below); amd64 does it in one VPMOVSXBD.
//
// The main loop is sixteen samples hardcoded with a sixty-four-sample step,
// tuned on an M2 Pro: sixty-four beats thirty-two by 9-15% up to 16K samples,
// and the step keeps lengths that are not a multiple of the block off the
// scalar loop. Writing the block longhand rather than a sixteen-sample helper
// is worth 3-5%, since the helper exceeds the inliner's budget.
//
// Against the NEON assembly a default build lands on this is 6-17% faster from
// 256 to 16K samples and 17% slower at 64K, where the assembly's PRFM -- which
// archsimd cannot express -- pays off. Neither wins at every size, so do not
// delete the assembly on the strength of the mid-range numbers.
func U8ToF32(dst []float32, src []byte) {
	n := min(len(src), len(dst))
	src, dst = src[:n], dst[:n]
	bias := archsimd.BroadcastUint8x16(128)
	for len(dst) >= 64 {
		in := (*[64]byte)(src[0:64])
		out := (*[64]float32)(dst[0:64])
		b0 := archsimd.LoadUint8x16Array((*[16]uint8)(in[0:16])).Sub(bias).BitsToInt8()
		lo0 := b0.ExtendLo8ToInt16()
		hi0 := b0.HiToLo().ExtendLo8ToInt16()
		lo0.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[0:4]))
		lo0.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[4:8]))
		hi0.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[8:12]))
		hi0.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[12:16]))
		b16 := archsimd.LoadUint8x16Array((*[16]uint8)(in[16:32])).Sub(bias).BitsToInt8()
		lo16 := b16.ExtendLo8ToInt16()
		hi16 := b16.HiToLo().ExtendLo8ToInt16()
		lo16.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[16:20]))
		lo16.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[20:24]))
		hi16.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[24:28]))
		hi16.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[28:32]))
		b32 := archsimd.LoadUint8x16Array((*[16]uint8)(in[32:48])).Sub(bias).BitsToInt8()
		lo32 := b32.ExtendLo8ToInt16()
		hi32 := b32.HiToLo().ExtendLo8ToInt16()
		lo32.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[32:36]))
		lo32.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[36:40]))
		hi32.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[40:44]))
		hi32.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[44:48]))
		b48 := archsimd.LoadUint8x16Array((*[16]uint8)(in[48:64])).Sub(bias).BitsToInt8()
		lo48 := b48.ExtendLo8ToInt16()
		hi48 := b48.HiToLo().ExtendLo8ToInt16()
		lo48.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[48:52]))
		lo48.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[52:56]))
		hi48.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[56:60]))
		hi48.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[60:64]))
		src, dst = src[64:], dst[64:]
	}
	for len(dst) >= 16 {
		in := (*[16]byte)(src[0:16])
		out := (*[16]float32)(dst[0:16])
		b0 := archsimd.LoadUint8x16Array((*[16]uint8)(in[0:16])).Sub(bias).BitsToInt8()
		lo0 := b0.ExtendLo8ToInt16()
		hi0 := b0.HiToLo().ExtendLo8ToInt16()
		lo0.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[0:4]))
		lo0.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[4:8]))
		hi0.ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[8:12]))
		hi0.HiToLo().ExtendLo4ToInt32().ConvertToFloat32().StoreArray((*[4]float32)(out[12:16]))
		src, dst = src[16:], dst[16:]
	}
	for i, v := range src {
		dst[i] = float32(int(v) - 128)
	}
}
