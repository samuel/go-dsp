package sampleio

import (
	"github.com/samuel/go-dsp/dsp"
	"github.com/samuel/go-dsp/dsp/encoding"
)

// decode fills dst with samples decoded from the front of src, normalized to
// full scale, and returns how many it wrote: min(len(dst), len(src)/f.Width()).
// A trailing partial sample in src is left alone.
//
// It decodes a contiguous run; interleaving is the Reader's problem.
//
// Two things are added to dsp's conversions, which do not scale:
//
// Byte order. A big-endian format is reversed into little-endian and handed to
// the same conversion, so there is one reversal loop rather than a second set of
// decoders. It costs about four times the little-endian path, 500 MB/s against
// 2.2 GB/s, still an order of magnitude past what a disk delivers.
//
// Normalization. dsp.VScale scales the conversion's output to full scale. The
// divisor is a power of two, so the pass costs no accuracy.
//
// swap is scratch for the byte reversal, grown as needed and untouched for a
// little-endian format. It may be nil, which costs an allocation per call and
// is fine for a one-shot caller but not for a Reader.
func (f Format) decode(dst []float64, src []byte, swap *[]byte) int {
	width := f.Width()
	if width == 0 {
		return 0
	}
	n := min(len(dst), len(src)/width)
	if n == 0 {
		return 0
	}
	dst = dst[:n]

	le, big := f.littleEndian()
	if big {
		// Reverse into scratch, never in the caller's buffer.
		src = reverseInto(swap, src, n, width)
	}

	switch le {
	case U8:
		encoding.U8ToF64(dst, src)
	case I8:
		encoding.I8ToF64(dst, src)
	case I16LE:
		encoding.I16LEToF64(dst, src)
	case I24LE:
		encoding.I24LEToF64(dst, src)
	case I32LE:
		encoding.I32LEToF64(dst, src)
	case F32LE:
		encoding.F32LEToF64(dst, src)
	case F64LE:
		encoding.F64LEToF64(dst, src)
	default:
		return 0
	}

	if s := f.fullScale(); s != 1 {
		dsp.VScale(dst, dst, s)
	}
	return n
}

// fullScale is the divisor, as a multiplier, that puts a sample from dsp's
// unscaled conversions onto [-1, 1). A float format arrives in those units
// already.
func (f Format) fullScale() float64 {
	_, _, _, bits := f.info()
	if bits == 0 {
		return 1
	}
	return 1 / float64(int64(1)<<(bits-1))
}

// reverseInto copies n samples of width bytes, reversing each sample's bytes,
// and returns the copy. One loop here replaces a second set of decoders.
func reverseInto(buf *[]byte, src []byte, n, width int) []byte {
	need := n * width
	var out []byte
	if buf == nil {
		out = make([]byte, need)
	} else {
		if cap(*buf) < need {
			*buf = make([]byte, need)
		}
		out = (*buf)[:need]
	}
	for i := range n {
		s := src[i*width:]
		d := out[i*width:]
		for j := range width {
			d[j] = s[width-1-j]
		}
	}
	return out
}

// littleEndian returns the little-endian format holding the same element type,
// and whether f was big-endian and so needs its bytes reversed first.
func (f Format) littleEndian() (Format, bool) {
	switch f {
	case I16BE:
		return I16LE, true
	case I24BE:
		return I24LE, true
	case I32BE:
		return I32LE, true
	case F32BE:
		return F32LE, true
	case F64BE:
		return F64LE, true
	}
	return f, false
}
