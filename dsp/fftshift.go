package dsp

import "slices"

// FFTShift rotates a transform in place so that the zero-frequency bin moves to
// the middle.
//
// It is the order a two-sided spectrum is drawn in. A forward transform returns
// the positive frequencies first and the negative ones folded onto the top
// half, so bin i is frequency i*rate/n for i below n/2 and (i-n)*rate/n above
// it. After shifting, bin i is (i-n/2)*rate/n, running monotonically from
// -rate/2 through 0 to just under +rate/2.
//
// This is a rotation of the caller's slice, not a spectrum-specific operation,
// so it applies equally to the src of an inverse transform -- and to a real
// slice, which is why the constraint is Sample rather than Complex.
func FFTShift[T Sample](dst []T) { shift(dst, len(dst)/2) }

// IFFTShift undoes FFTShift.
//
// For an even length the two are the same rotation and each is its own inverse,
// so the distinction only matters for the odd lengths DFT accepts.
func IFFTShift[T Sample](dst []T) { shift(dst, len(dst)-len(dst)/2) }

// shift rotates x right by k places, in place.
func shift[T Sample](dst []T, k int) {
	n := len(dst)
	if n == 0 {
		return
	}
	if k %= n; k == 0 {
		return
	}
	if 2*k == n {
		// Every transform length here is a power of two, so this is the case
		// that actually runs: the rotation is a straight swap of the two halves,
		// one pass over the slice rather than the three the general form takes.
		for i := range k {
			dst[i], dst[i+k] = dst[i+k], dst[i]
		}
		return
	}
	// Reverse the whole slice, then each of the two pieces the rotation splits
	// it into. No scratch buffer, and no walking of the cycles a gcd-based
	// rotation would need.
	slices.Reverse(dst)
	slices.Reverse(dst[:k])
	slices.Reverse(dst[k:])
}
