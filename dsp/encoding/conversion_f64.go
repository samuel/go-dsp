package encoding

import (
	"encoding/binary"
	"math"
)

// The float64 destinations, one per sample encoding.
//
// None of them scales; each integer sample stays at its own full scale — the
// 8-bit forms run -128 to 127. A caller wanting [-1, 1) divides by 2^(bits-1),
// a power of two, so it costs no accuracy wherever applied; the cheap place is
// the coefficients of the next linear stage rather than a dsp.VScale pass. See
// the package comment.

// U8ToF64 converts unsigned 8-bit samples to float64, subtracting the 128 bias
// as U8ToF32 does. It does not scale: the range is -128 to 127.
func U8ToF64(dst []float64, src []byte) {
	n := min(len(src), len(dst))
	for i, v := range src[:n] {
		dst[i] = float64(int8(v - 128))
	}
}

// I8ToF64 converts signed 8-bit samples to float64. The slice is []byte,
// matching I8ToF32. It does not scale.
func I8ToF64(dst []float64, src []byte) {
	n := min(len(src), len(dst))
	for i, v := range src[:n] {
		dst[i] = float64(int8(v))
	}
}

// I16LEToF64 converts little-endian int16 bytes to float64. It does not scale:
// the range is -32768 to 32767.
func I16LEToF64(dst []float64, src []byte) { i16LEToF64Asm(dst, src) }
func i16LEToF64(dst []float64, src []byte) {
	n := min(len(src)/2, len(dst))
	for i := range dst[:n] {
		dst[i] = float64(int16(uint16(src[i*2]) | (uint16(src[i*2+1]) << 8)))
	}
}

// I24LEToF64 converts packed 24-bit signed samples stored little endian to
// float64. It does not scale: the range is -8388608 to 8388607.
//
// The samples are three unpadded bytes each, as a 24-bit WAV stores them.
func I24LEToF64(dst []float64, src []byte) {
	n := min(len(src)/3, len(dst))
	for i := range dst[:n] {
		b := src[i*3:]
		dst[i] = float64(sext24(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16))
	}
}

// I32LEToF64 converts little-endian int32 bytes to float64. It does not scale.
//
// Unlike I32LEToF32 this is exact: a 32-bit integer does not fit float32's
// 24-bit mantissa, so the narrower conversion rounds low bits away.
func I32LEToF64(dst []float64, src []byte) {
	n := min(len(src)/4, len(dst))
	for i := range dst[:n] {
		dst[i] = float64(int32(binary.LittleEndian.Uint32(src[i*4:])))
	}
}

// F32LEToF64 reads little-endian float32 bytes and widens them to float64.
// Widening is exact, so nothing is lost or invented.
func F32LEToF64(dst []float64, src []byte) {
	n := min(len(src)/4, len(dst))
	for i := range dst[:n] {
		dst[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(src[i*4:])))
	}
}

// F64LEToF64 reads little-endian float64 bytes: the inverse of the float64
// half of F32ToF32LE's family. The bytes need not be aligned.
func F64LEToF64(dst []float64, src []byte) {
	n := min(len(src)/8, len(dst))
	for i := range dst[:n] {
		dst[i] = math.Float64frombits(binary.LittleEndian.Uint64(src[i*8:]))
	}
}

// sext24 sign-extends a 24-bit two's complement value.
func sext24(v uint32) int32 {
	if v&0x800000 != 0 {
		v |= 0xff000000
	}
	return int32(v)
}
