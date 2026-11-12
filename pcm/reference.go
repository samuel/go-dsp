package pcm

import (
	"encoding/binary"
	"math"
)

// The Go reference kernels. Each one defines the result of a conversion
// between a format and a float width, including the scaling. An assembly
// kernel must match its reference bit for bit, and an architecture with no
// assembly for a kernel runs the reference.
//
// A kernel works on little-endian bytes. A decoder handles
// min(len(dst), len(src)/width) samples and an encoder
// min(len(dst)/width, len(src)).
//
// Decoding multiplies an integer by 1/2^(bits-1), a power of two, so the
// multiply is exact. An integer below 2^24 converts to float32 exactly. A
// larger one is rounded once, by the conversion, the same way on every
// architecture.
//
// Each loop takes its sample with a full slice expression, src[lo:hi:hi].
// That lets the compiler remove the bounds checks inside the loop, which
// makes it about twice as fast as indexing src[i*w:].

type float interface{ float32 | float64 }

func u8ToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = F(int(v)-128) * (1.0 / (1 << 7))
	}
}

func i8ToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = F(int8(v)) * (1.0 / (1 << 7))
	}
}

func i16LEToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src)/2)
	for i := range n {
		s := src[i*2 : i*2+2 : i*2+2]
		dst[i] = F(int16(binary.LittleEndian.Uint16(s))) * (1.0 / (1 << 15))
	}
}

func i24LEToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src)/3)
	for i := range n {
		s := src[i*3 : i*3+3 : i*3+3]
		v := int32(uint32(s[0])<<8|uint32(s[1])<<16|uint32(s[2])<<24) >> 8
		dst[i] = F(v) * (1.0 / (1 << 23))
	}
}

func i32LEToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src)/4)
	for i := range n {
		s := src[i*4 : i*4+4 : i*4+4]
		dst[i] = F(int32(binary.LittleEndian.Uint32(s))) * (1.0 / (1 << 31))
	}
}

// f32LEToFloatRef copies a float32 unchanged, including the payload of a NaN,
// or widens it to float64, which is exact.
func f32LEToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src)/4)
	for i := range n {
		s := src[i*4 : i*4+4 : i*4+4]
		dst[i] = F(math.Float32frombits(binary.LittleEndian.Uint32(s)))
	}
}

// f64LEToFloatRef copies a float64 unchanged, or narrows it to float32,
// rounding to nearest even.
func f64LEToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src)/8)
	for i := range n {
		s := src[i*8 : i*8+8 : i*8+8]
		dst[i] = F(math.Float64frombits(binary.LittleEndian.Uint64(s)))
	}
}

// roundBias rounds a float64 to an integer, with ties to even, when it is used
// as (v + roundBias) - roundBias and v is below 2^51 in magnitude. The sum is
// too large to hold a fraction, so the addition rounds v to an integer, and
// the subtraction is then exact.
//
// Every integer encoder rounds this way, so the result does not depend on how
// an architecture's float-to-integer conversion rounds.
const roundBias = 1.5 * (1 << 52)

// quantize returns v rounded to the nearest integer, with ties to even, and
// clipped to [lo, hi]. NaN gives zero. It clips first: lo and hi are
// integers, so the order does not change the result, and the clipped value is
// small enough for roundBias.
func quantize(v, lo, hi float64) int32 {
	switch {
	case v >= hi:
		return int32(hi)
	case v <= lo:
		return int32(lo)
	case v == v: // false only for NaN
		return int32((v + roundBias) - roundBias)
	}
	return 0
}

// The multiply by 2^(bits-1) is done in float64, where it is exact for a
// float32 or a float64 sample. A float32 sample therefore encodes to the same
// code as the same value held in a float64.

func floatToU8Ref[F float](dst []byte, src []F) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = byte(quantize(float64(v)*(1<<7), -(1<<7), 1<<7-1) + 128)
	}
}

func floatToI8Ref[F float](dst []byte, src []F) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = byte(quantize(float64(v)*(1<<7), -(1 << 7), 1<<7-1))
	}
}

func floatToI16LERef[F float](dst []byte, src []F) {
	n := min(len(dst)/2, len(src))
	for i, v := range src[:n] {
		d := dst[i*2 : i*2+2 : i*2+2]
		binary.LittleEndian.PutUint16(d, uint16(quantize(float64(v)*(1<<15), -(1<<15), 1<<15-1)))
	}
}

func floatToI24LERef[F float](dst []byte, src []F) {
	n := min(len(dst)/3, len(src))
	for i, v := range src[:n] {
		q := quantize(float64(v)*(1<<23), -(1 << 23), 1<<23-1)
		d := dst[i*3 : i*3+3 : i*3+3]
		d[0], d[1], d[2] = byte(q), byte(q>>8), byte(q>>16)
	}
}

func floatToI32LERef[F float](dst []byte, src []F) {
	n := min(len(dst)/4, len(src))
	for i, v := range src[:n] {
		d := dst[i*4 : i*4+4 : i*4+4]
		binary.LittleEndian.PutUint32(d, uint32(quantize(float64(v)*(1<<31), -(1<<31), 1<<31-1)))
	}
}

// floatToF32LERef writes a float32 unchanged, or narrows a float64 to
// float32, rounding to nearest even. It does not clip, and NaN stays NaN.
func floatToF32LERef[F float](dst []byte, src []F) {
	n := min(len(dst)/4, len(src))
	for i, v := range src[:n] {
		d := dst[i*4 : i*4+4 : i*4+4]
		binary.LittleEndian.PutUint32(d, math.Float32bits(float32(v)))
	}
}

func floatToF64LERef[F float](dst []byte, src []F) {
	n := min(len(dst)/8, len(src))
	for i, v := range src[:n] {
		d := dst[i*8 : i*8+8 : i*8+8]
		binary.LittleEndian.PutUint64(d, math.Float64bits(float64(v)))
	}
}
