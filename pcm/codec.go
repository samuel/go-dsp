package pcm

import (
	"samuelks.com/dsp"
	"samuelks.com/dsp/internal/view"
)

// Decode converts the bytes of src, in format f, to full-scale samples in dst.
// It returns the number of samples it wrote, which is
// min(len(dst), len(src)/size). size is f.Width() when T is real and twice
// that when T is complex, since a complex sample is read as an I value then a
// Q value. Bytes at the end of src that do not make a whole sample are
// ignored. A format that is not valid decodes nothing.
//
// How a sample is converted depends on the format:
//
//   - An integer is divided by 2^(bits-1), after 128 is subtracted from a U8.
//     The most negative code gives exactly -1 and the most positive gives one
//     step less than 1. The division is by a power of two, so the result is
//     exact whenever T can hold it, and is the same on every architecture. The
//     one case that reaches 1 is I32 into float32, where the largest codes
//     round up to it.
//   - A float is not scaled. It is widened or narrowed to T.
//   - A G.711 code gives its level on the 16-bit scale divided by 2^15, which
//     is exact. The largest level is below full scale.
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

// Encode converts the full-scale samples of src to bytes in dst, in format f.
// It returns the number of samples it wrote, which is
// min(len(dst)/size, len(src)), with size as for Decode. Bytes of dst after
// the last whole sample are not changed. A format that is not valid encodes
// nothing.
//
// How a sample is converted depends on the format:
//
//   - For an integer format the sample is multiplied by 2^(bits-1), rounded to
//     the nearest integer with ties to even, and clipped to the format's
//     range. NaN gives zero. A sample of 1 or more gives the largest code.
//   - For a float format the sample is not scaled or clipped, and NaN stays
//     NaN. A float64 written as F32 is rounded to nearest even.
//   - For a G.711 format the sample is rounded to 16 bits as for I16, and the
//     result is given the code of the G.711 step it falls in.
//
// Decoding bytes and encoding the result gives back the same bytes for every
// integer code, and for every G.711 code except mu-law's negative zero, 0x7f,
// which comes back as 0xff.
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

// reverseBlock is the size of the stack buffer used for a big-endian format.
// Each sample's bytes are reversed into or out of it, so that the kernel only
// sees little-endian samples.
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

// bigEndian reports whether f stores the most significant byte of a sample
// first. Such a format has its bytes reversed around the little-endian
// kernel.
func (f Format) bigEndian() bool {
	switch f {
	case I16BE, I24BE, I32BE, F32BE, F64BE:
		return true
	}
	return false
}

// The four functions below return the kernel for a format. Both byte orders
// share the little-endian kernel. A format that is not valid returns nil.

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
	case ALaw:
		return alawToF32
	case MuLaw:
		return mulawToF32
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
	case ALaw:
		return alawToF64
	case MuLaw:
		return mulawToF64
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
	case ALaw:
		return f32ToALaw
	case MuLaw:
		return f32ToMuLaw
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
	case ALaw:
		return f64ToALaw
	case MuLaw:
		return f64ToMuLaw
	}
	return nil
}
