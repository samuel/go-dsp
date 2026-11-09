package dsp

import (
	"errors"
	"math"
	"math/bits"
)

// FFT is a fixed-size fast Fourier transform. It precomputes the twiddle
// factors and the bit-reversal permutation for one transform length and reuses
// them, so build one per length and call it as often as needed.
//
// The sample type cannot be inferred from the constructor's arguments and has
// to be given, as in NewFFT[complex128](1024).
//
// Forward and Inverse hold no state, but ForwardReal allocates a widening
// buffer on its first call and reuses it after that, so an FFT is not safe for
// concurrent use. Build one per goroutine; the twiddles, the expensive part,
// are only a slice of n/2.
//
// Use complex128 for anything that must resolve more than about 120 dB:
// complex64 carries 24 bits of mantissa, so a transform of any useful length
// has a noise floor above the stopband of a good filter.
type FFT[T Complex] struct {
	n   int
	tw  []T     // n/2 forward twiddles, exp(-2*pi*i*k/n)
	rev []int32 // bit-reversal permutation

	buf []T // lazily allocated widening buffer, for ForwardReal
}

// NewFFT returns a transform of n points, which must be a positive power of
// two. Bluestein's algorithm would lift that restriction but is deliberately
// not here: DFT and DFTBin cover an awkward length exactly, and evaluating the
// few bins that matter is cheaper and more accurate than a padded transform of
// the whole block.
func NewFFT[T Complex](n int) (*FFT[T], error) {
	if n <= 0 || n&(n-1) != 0 {
		return nil, errors.New("dsp: FFT length must be a positive power of two")
	}
	f := &FFT[T]{n: n, rev: make([]int32, n)}

	// Each twiddle is computed from its own angle rather than by advancing a
	// running product, which would drift off the unit circle by roughly n*eps
	// and put a floor under every measurement made through this transform.
	if n > 1 {
		f.tw = make([]T, n/2)
		for k := range f.tw {
			s, c := math.Sincos(-2 * math.Pi * float64(k) / float64(n))
			f.tw[k] = T(complex(c, s))
		}
	}

	logn := bits.TrailingZeros(uint(n))
	for i := range f.rev {
		f.rev[i] = int32(bits.Reverse(uint(i)) >> (bits.UintSize - logn))
	}
	return f, nil
}

// Len returns the transform length.
func (f *FFT[T]) Len() int { return f.n }

// Forward writes the forward transform of src to dst. Both must hold at
// least Len elements, and they may be the same slice. Anything past Len is
// ignored.
//
// The transform is decimation in time, so it accumulates about log2(n)
// roundings: in complex128 a 65536-point transform lands near -307 dB, spread
// over all n bins. In complex64 the same transform lands near -124 dB, above
// the stopband of most filters worth measuring, so complex64 is for signal
// processing rather than for analysis.
func (f *FFT[T]) Forward(dst, src []T) {
	f.transform(dst, src)
}

// Inverse writes the inverse transform of src to dst, scaled by 1/n so
// that Inverse after Forward returns the original signal. The same length and
// aliasing rules as Forward apply.
func (f *FFT[T]) Inverse(dst, src []T) {
	n := f.n
	f.transform(dst, src)

	// The inverse is the forward transform read backwards: substituting
	// exp(+2*pi*i*k*j/n) = exp(-2*pi*i*k*(n-j)/n) gives x[j] = y[(n-j) mod n]/n,
	// so reversing bins 1..n-1 is the whole of it and there is one butterfly and
	// one twiddle table rather than two.
	for i, j := 1, n-1; i < j; i, j = i+1, j-1 {
		dst[i], dst[j] = dst[j], dst[i]
	}
	scale := T(complex(1/float64(n), 0))
	for i := range n {
		dst[i] *= scale
	}
}

func (f *FFT[T]) transform(dst, src []T) {
	n := f.n
	if len(src) < n || len(dst) < n {
		panic("dsp: FFT src and dst must hold at least Len elements")
	}

	// copy first, then permute in place, so src and dst may be the same
	// slice. The permutation is an involution, so swapping each pair once is
	// enough.
	copy(dst[:n], src[:n])
	for i := range n {
		if j := int(f.rev[i]); i < j {
			dst[i], dst[j] = dst[j], dst[i]
		}
	}

	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		step := n / size
		for i := 0; i < n; i += size {
			k := 0
			for j := i; j < i+half; j++ {
				u := dst[j]
				v := dst[j+half] * f.tw[k]
				dst[j] = u + v
				dst[j+half] = u - v
				k += step
			}
		}
	}
}

// ForwardReal writes the transform of a real signal to dst, widening it into the
// complex transform f. It is a function rather than a method so that both type
// arguments are inferred: the complex width from f and the real width from src.
//
// src shorter than f.Len is zero-padded, which is how a window's sidelobes
// are resolved: pad by a factor of sixteen and the transform interpolates the
// window's continuous spectrum finely enough to find the peak between bins.
// dst receives all n bins; for a real src they are conjugate-symmetric, so
// the caller usually reads only the first n/2+1.
//
// It allocates a buffer of n complex values on its first call and keeps it, so
// unlike Forward and Inverse it writes to f. Two goroutines must not share one
// FFT because of it.
func ForwardReal[F Float, C Complex](f *FFT[C], dst []C, src []F) {
	n := f.n
	if f.buf == nil {
		f.buf = make([]C, n)
	}
	buf := f.buf
	for i := range n {
		var v float64
		if i < len(src) {
			v = float64(src[i])
		}
		buf[i] = C(complex(v, 0))
	}
	f.transform(dst, buf)
}

// DFT writes the discrete Fourier transform of src to dst, evaluating the
// sum directly. It costs O(n^2) where FFT costs O(n log n), and accepts any
// length. It is the definition the fast transform is checked against, and the
// way to transform a length the fast one cannot.
//
// Like the conversions, it processes the smaller of the two slices, and the two
// may be the same slice -- but not partially overlapping ones, which is the
// aliasing rule everywhere in this package.
func DFT[T Complex](dst, src []T) {
	n := min(len(src), len(dst))
	if n == 0 {
		return
	}
	// Every bin reads the whole of src, so when dst and src are the same slice
	// the bins must not be written as they are computed; separate slices skip
	// an allocation and a copy they never needed.
	out, aliased := dst[:n], &dst[0] == &src[0]
	if aliased {
		out = make([]T, n)
	}
	for k := range out {
		out[k] = dftBin(src[:n], k)
	}
	if aliased {
		copy(dst[:n], out)
	}
}

// DFTBinReal returns bin k of the len(src) point transform of a real signal,
// scaled so that a cosine of amplitude a completing exactly k cycles over src
// comes back as a.
//
// It is DFTBin plus that scaling: a real cosine splits evenly between bins k
// and n-k, so the bin itself carries half of it, and the factor of two here
// undoes that. The length need not be a power of two, and the phase argument is
// reduced in integers for the reason dftBin gives, so a coherent measurement is
// limited by the arithmetic rather than by the correlation.
func DFTBinReal[F Float](src []F, k int) complex128 {
	n := len(src)
	if n == 0 {
		return 0
	}
	var re, im float64
	for i, v := range src {
		m := int64(k) * int64(i) % int64(n)
		s, c := math.Sincos(2 * math.Pi * float64(m) / float64(n))
		re += float64(v) * c
		im -= float64(v) * s
	}
	return 2 * complex(re, im) / complex(float64(n), 0)
}

// DFTBin returns bin k of the len(src) point transform of src, for any
// length and any k. Bins repeat every len(src), so k may be negative or
// larger than src.
//
// Unlike Goertzel it evaluates the sum term by term with exact twiddles rather
// than by a recursion, so it does not lose accuracy over a long block, and k is
// not snapped to the bins of a fixed block size. It costs the same O(n).
func DFTBin[T Complex](src []T, k int) T {
	if len(src) == 0 {
		var zero T
		return zero
	}
	return dftBin(src, k)
}

func dftBin[T Complex](src []T, k int) T {
	n := len(src)

	// Reduce the index into [0, n) and form k*i in integers, so the angle is
	// always a ratio of small exact quantities. Computing 2*pi*k*i/n in floating
	// point loses the low bits once k*i passes 2^53/(2*pi), and the error is
	// patterned rather than white: spurious tones, not rounding noise.
	k %= n
	if k < 0 {
		k += n
	}
	// int64 because k*i overflows a 32-bit int for n past 2^16.
	var sum complex128
	for i, x := range src {
		m := int64(k) * int64(i) % int64(n)
		s, c := math.Sincos(-2 * math.Pi * float64(m) / float64(n))
		sum += complex128(x) * complex(c, s)
	}
	return T(sum)
}
