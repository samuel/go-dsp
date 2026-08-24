package encoding

import (
	"encoding/binary"
	"math"
)

//go:generate go run conversion_avo_amd64.go -out conversion_avo_amd64.s -stubs conversion_avo_stub_amd64.go

// U8ToI16 converts unsigned 8-bit samples to 16-bit signed, spreading each
// byte over the full 16-bit range: the biased byte is replicated into both
// halves, mapping 0 to -32640 and 255 to 32639.
func U8ToI16(dst []int16, src []byte) { u8ToI16Asm(dst, src) }
func u8ToI16(dst []int16, src []byte) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		v -= 128
		v16 := int16((uint16(v) << 8) | uint16(v))
		dst[i] = v16
	}
}

// U8ToI16LE is U8ToI16 with dst as little-endian bytes.
func U8ToI16LE(dst, src []byte) { u8ToI16LEAsm(dst, src) }
func u8ToI16LE(dst, src []byte) {
	n := min(len(dst)/2, len(src))
	for i, v := range src[:n] {
		v -= 128
		dst[i*2] = v
		dst[i*2+1] = v
	}
}

// I8ToF32 converts signed 8-bit samples to float32. It does not scale.
func I8ToF32(dst []float32, src []byte) { i8ToF32Asm(dst, src) }
func i8ToF32(dst []float32, src []byte) {
	n := min(len(src), len(dst))
	for i, v := range src[:n] {
		dst[i] = float32(int8(v))
	}
}

// I8ToC64 converts signed 8-bit interleaved complex samples to complex64
// (32-bit parts). The samples are signed but the slice is []byte, matching
// I8ToF32 and the []byte an SDR hands over.
func I8ToC64(dst []complex64, src []byte) {
	n := min(len(src)/2, len(dst))
	for i := range n {
		dst[i] = complex(
			float32(int8(src[i*2])),
			float32(int8(src[i*2+1])),
		)
	}
}

// C64ToI8 converts complex64 samples to signed 8-bit interleaved, in a []byte
// as I8ToC64 reads them. It does not scale: values past full scale wrap.
func C64ToI8(dst []byte, src []complex64) {
	n := min(len(src), len(dst)/2)
	for i, s := range src[:n] {
		dst[i*2] = byte(int8(real(s)))
		dst[i*2+1] = byte(int8(imag(s)))
	}
}

// F32ToI16 converts float32 samples to int16. It does not scale: a signal
// running [-1, 1) must reach full scale first, by a factor of 1<<15 folded into
// the last stage that produced it, or by dsp.VScale where nothing upstream can
// carry it. The clip below is nonlinear, so the factor cannot simply move
// downstream instead. Samples past full scale clip to the rails rather than
// wrap, and NaN converts to zero.
func F32ToI16(dst []int16, src []float32) { f32ToI16Asm(dst, src) }
func f32ToI16(dst []int16, src []float32) {
	n := min(len(src), len(dst))
	for i, v := range src[:n] {
		dst[i] = clipToI16(v)
	}
}

// F32ToI16LE writes F32ToI16 results as little-endian int16 bytes; it clips
// the same way.
func F32ToI16LE(dst []byte, src []float32) { f32ToI16LEAsm(dst, src) }
func f32ToI16LE(dst []byte, src []float32) {
	n := min(len(src), len(dst)/2)
	for i, v := range src[:n] {
		v := uint16(clipToI16(v))
		dst[i*2] = uint8(v & 0xff)
		dst[i*2+1] = uint8(v >> 8)
	}
}

// clipToI16 converts v to an int16, saturating at the rails.
//
// Go's float-to-int conversion is implementation-defined out of range, so a
// bare int16(v) neither clips nor wraps predictably — arm64 folds it to the
// opposite rail, turning overdrive into full-amplitude noise. The comparisons
// come first. NaN fails both and converts to zero, matching the ARM and NEON
// saturating converts.
func clipToI16(v float32) int16 {
	switch {
	case v > math.MaxInt16:
		return math.MaxInt16
	case v < math.MinInt16:
		return math.MinInt16
	case v != v:
		return 0
	}
	return int16(v)
}

// I16ToI16LE writes int16 values as little-endian bytes.
func I16ToI16LE(dst []byte, src []int16) { i16ToI16LEAsm(dst, src) }
func i16ToI16LE(dst []byte, src []int16) {
	n := min(len(src), len(dst)/2)
	for i, v := range src[:n] {
		dst[i*2] = byte(v & 0xff)
		dst[i*2+1] = byte(v >> 8)
	}
}

// I16LEToF32 converts little-endian int16 bytes to float32. It does not scale.
func I16LEToF32(dst []float32, src []byte) { i16LEToF32Asm(dst, src) }
func i16LEToF32(dst []float32, src []byte) {
	n := min(len(src)/2, len(dst))
	for i := range dst[:n] {
		dst[i] = float32(int16(uint16(src[i*2]) | (uint16(src[i*2+1]) << 8)))
	}
}

// I32LEToF32 converts little-endian int32 bytes to float32. It does not scale.
func I32LEToF32(dst []float32, src []byte) {
	n := min(len(src)/4, len(dst))
	for i := range dst[:n] {
		dst[i] = float32(
			int32(
				uint32(src[i*4]) |
					(uint32(src[i*4+1]) << 8) |
					(uint32(src[i*4+2]) << 16) |
					(uint32(src[i*4+3]) << 24)))
	}
}

// F32ToF32LE writes float32 values as little-endian float32 bytes.
func F32ToF32LE(dst []byte, src []float32) {
	n := min(len(src), len(dst)/4)
	for i, s := range src[:n] {
		binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(s))
	}
}
