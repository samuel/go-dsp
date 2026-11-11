package pcm

import (
	"samuelks.com/dsp"
	"samuelks.com/dsp/internal/view"
)

// Decode fills dst with samples of format f from the front of src, at full
// scale, and returns how many it wrote: min(len(dst), len(src)/size), where
// size is f.Width() for a real T and twice that for a complex one, which reads
// interleaved I/Q pairs. A trailing partial sample in src is left alone, and an
// invalid f decodes nothing.
//
// An integer sample is divided by 2^(bits-1), so the most negative code is
// exactly -1 and the most positive one step short of +1: [-1, 1), except that
// I32 into float32 rounds its top codes up to exactly 1. The divisor is a
// power of two, so the result is exact wherever T can hold it, and the same on
// every architecture. U8 has its bias of 128 removed first. A float format
// passes through unscaled, widened or narrowed to T.
func Decode[T dsp.Sample](dst []T, src []byte, f Format) int {
	switch d := any(dst).(type) {
	case []float32:
		return decode(d, src, f, decoderF32(f))
	case []float64:
		return decode(d, src, f, decoderF64(f))
	case []complex64:
		n := min(len(d), len(src)/max(2*f.Width(), 1))
		return decode(view.C64ToF32(d[:n]), src, f, decoderF32(f)) / 2
	case []complex128:
		n := min(len(d), len(src)/max(2*f.Width(), 1))
		return decode(view.C128ToF64(d[:n]), src, f, decoderF64(f)) / 2
	default:
		panic("pcm: Decode has no kernel for this sample type")
	}
}

// Encode writes the samples of src into dst in format f and returns how many
// it wrote: min(len(dst)/size, len(src)), with size as for Decode. Bytes of dst
// past the last whole sample are left alone, and an invalid f encodes nothing.
//
// An integer format takes each sample times 2^(bits-1), rounded to nearest
// even, clipped to the format's range, with NaN giving zero: +1.0 gives the top
// code, and Encode(Decode(b)) is b for every integer code. A float format
// passes the sample through, widened or rounded to nearest even, with no clip
// and NaN kept.
func Encode[T dsp.Sample](dst []byte, src []T, f Format) int {
	switch s := any(src).(type) {
	case []float32:
		return encode(dst, s, f, encoderF32(f))
	case []float64:
		return encode(dst, s, f, encoderF64(f))
	case []complex64:
		n := min(len(dst)/max(2*f.Width(), 1), len(s))
		return encode(dst, view.C64ToF32(s[:n]), f, encoderF32(f)) / 2
	case []complex128:
		n := min(len(dst)/max(2*f.Width(), 1), len(s))
		return encode(dst, view.C128ToF64(s[:n]), f, encoderF64(f)) / 2
	default:
		panic("pcm: Encode has no kernel for this sample type")
	}
}

// reverseBlock is the stack buffer a big-endian format's bytes are reversed
// through, in front of the little-endian kernel.
const reverseBlock = 4096

func decode[F float](dst []F, src []byte, f Format, kernel func([]F, []byte)) int {
	width := f.Width()
	if kernel == nil || width == 0 {
		return 0
	}
	n := min(len(dst), len(src)/width)
	if !f.bigEndian() {
		kernel(dst[:n], src[:n*width])
		return n
	}
	var buf [reverseBlock]byte
	per := len(buf) / width
	for i := 0; i < n; i += per {
		m := min(per, n-i)
		block := buf[:m*width]
		reverseInto(block, src[i*width:], m, width)
		kernel(dst[i:i+m], block)
	}
	return n
}

func encode[F float](dst []byte, src []F, f Format, kernel func([]byte, []F)) int {
	width := f.Width()
	if kernel == nil || width == 0 {
		return 0
	}
	n := min(len(dst)/width, len(src))
	if !f.bigEndian() {
		kernel(dst[:n*width], src[:n])
		return n
	}
	var buf [reverseBlock]byte
	per := len(buf) / width
	for i := 0; i < n; i += per {
		m := min(per, n-i)
		block := buf[:m*width]
		kernel(block, src[i:i+m])
		reverseInto(dst[i*width:], block, m, width)
	}
	return n
}

// reverseInto copies n samples of width bytes from src to dst, reversing each
// sample's bytes.
func reverseInto(dst, src []byte, n, width int) {
	for i := range n {
		s := src[i*width : i*width+width]
		d := dst[i*width : i*width+width]
		for j := range width {
			d[j] = s[width-1-j]
		}
	}
}

// bigEndian reports whether f's samples are stored most significant byte
// first, and so need reversing around the little-endian kernel.
func (f Format) bigEndian() bool {
	switch f {
	case I16BE, I24BE, I32BE, F32BE, F64BE:
		return true
	}
	return false
}

// The kernel of each format's element type, ignoring byte order. Nil for an
// invalid format.

func decoderF32(f Format) func([]float32, []byte) {
	switch f {
	case U8:
		return u8ToF32
	case I8:
		return i8ToF32
	case I16LE, I16BE:
		return i16LEToF32
	case I24LE, I24BE:
		return i24LEToF32
	case I32LE, I32BE:
		return i32LEToF32
	case F32LE, F32BE:
		return f32LEToF32
	case F64LE, F64BE:
		return f64LEToF32
	}
	return nil
}

func decoderF64(f Format) func([]float64, []byte) {
	switch f {
	case U8:
		return u8ToF64
	case I8:
		return i8ToF64
	case I16LE, I16BE:
		return i16LEToF64
	case I24LE, I24BE:
		return i24LEToF64
	case I32LE, I32BE:
		return i32LEToF64
	case F32LE, F32BE:
		return f32LEToF64
	case F64LE, F64BE:
		return f64LEToF64
	}
	return nil
}

func encoderF32(f Format) func([]byte, []float32) {
	switch f {
	case U8:
		return f32ToU8
	case I8:
		return f32ToI8
	case I16LE, I16BE:
		return f32ToI16LE
	case I24LE, I24BE:
		return f32ToI24LE
	case I32LE, I32BE:
		return f32ToI32LE
	case F32LE, F32BE:
		return f32ToF32LE
	case F64LE, F64BE:
		return f32ToF64LE
	}
	return nil
}

func encoderF64(f Format) func([]byte, []float64) {
	switch f {
	case U8:
		return f64ToU8
	case I8:
		return f64ToI8
	case I16LE, I16BE:
		return f64ToI16LE
	case I24LE, I24BE:
		return f64ToI24LE
	case I32LE, I32BE:
		return f64ToI32LE
	case F32LE, F32BE:
		return f64ToF32LE
	case F64LE, F64BE:
		return f64ToF64LE
	}
	return nil
}
