package convert

import "samuelks.com/dsp/internal/f64"

// Decode fills dst with samples of format f decoded from the front of src,
// normalized to full scale, and returns how many it wrote:
// min(len(dst), len(src)/f.Width()). A trailing partial sample in src is left
// alone, and an invalid format decodes nothing.
//
// Unlike the conversions it scales: an integer sample is divided by
// 2^(bits-1), so the most negative code is exactly -1 and the most positive
// one step short of +1, which is what makes a full-scale sine read 0 dBFS.
// The divisor is a power of two, so the scaling costs no accuracy. A float
// format is already in those units and passes through.
//
// A big-endian format is reversed into little-endian and handed to the same
// conversion, in blocks through a buffer on the stack, so there is one
// reversal loop rather than a second set of decoders and no scratch for the
// caller to keep. It costs about four times the little-endian path, 500 MB/s
// against 2.2 GB/s, still an order of magnitude past what a disk delivers.
func Decode(dst []float64, src []byte, f Format) int {
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
	if !big {
		decodeLE(dst, src, le)
	} else {
		var buf [4096]byte
		per := len(buf) / width
		for i := 0; i < n; i += per {
			m := min(per, n-i)
			block := buf[:m*width]
			reverseInto(block, src[i*width:], m, width)
			decodeLE(dst[i:i+m], block, le)
		}
	}

	if bits := f.Bits(); bits != 0 {
		f64.Scale(dst, dst, 1/float64(int64(1)<<(bits-1)))
	}
	return n
}

// decodeLE runs the unscaled conversion for a little-endian or byte format.
func decodeLE(dst []float64, src []byte, f Format) {
	switch f {
	case U8:
		U8ToF64(dst, src)
	case I8:
		I8ToF64(dst, src)
	case I16LE:
		I16LEToF64(dst, src)
	case I24LE:
		I24LEToF64(dst, src)
	case I32LE:
		I32LEToF64(dst, src)
	case F32LE:
		F32LEToF64(dst, src)
	case F64LE:
		F64LEToF64(dst, src)
	}
}

// reverseInto copies n samples of width bytes from src to dst, reversing each
// sample's bytes.
func reverseInto(dst, src []byte, n, width int) {
	for i := range n {
		s := src[i*width:]
		d := dst[i*width:]
		for j := range width {
			d[j] = s[width-1-j]
		}
	}
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
