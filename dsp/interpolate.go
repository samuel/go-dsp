package dsp

import "math"

// The interpolators in this file all read a signal at a fractional position.
//
// x is measured in src: 0 is the first sample of src, 1 the second, and
// the fractional part selects between neighbors. Every function shares one
// out-of-range policy — a position before the first sample or after the last
// holds the nearest edge value, rather than fading to zero, which would put a
// step discontinuity at both ends of every buffer. An empty buffer, or an x
// that is NaN, gives zero.

// splitPos separates x into the index of the sample at or below it and the
// fraction between that sample and the next, reporting false for an empty
// buffer or a NaN x. Positions far outside the buffer are pinned to just
// outside it: converting an out-of-range float to int is implementation-defined
// in Go, so the infinities are kept away from the conversion.
func splitPos[T Float](n int, x T) (xi int, frac T, ok bool) {
	switch {
	case n == 0 || x != x: //nolint:gocritic // x != x is the NaN test; math.IsNaN takes a float64
		return 0, 0, false
	case x < -1:
		return -1, 0, true
	case x > T(n):
		return n, 0, true
	}
	fl := T(math.Floor(float64(x)))
	return int(fl), x - fl, true
}

// sampleAt returns src[i] with i clamped to the ends of the buffer.
func sampleAt[T Float](src []T, i int) T {
	if i < 0 {
		i = 0
	} else if i >= len(src) {
		i = len(src) - 1
	}
	return src[i]
}

// Linear interpolates between the two src either side of x.
func Linear[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	lo := sampleAt(src, xi)
	hi := sampleAt(src, xi+1)
	return lo + frac*(hi-lo)
}

// Hermite4p3o interpolates using 4-point, 3rd-order Hermite (x-form).
func Hermite4p3o[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	var s [4]T
	for i := range s {
		s[i] = sampleAt(src, xi-1+i)
	}
	c0 := s[1]
	c1 := 1.0 / 2.0 * (s[2] - s[0])
	c2 := s[0] - 5.0/2.0*s[1] + 2.0*s[2] - 1.0/2.0*s[3]
	c3 := 1.0/2.0*(s[3]-s[0]) + 3.0/2.0*(s[1]-s[2])
	return ((c3*frac+c2)*frac+c1)*frac + c0
}

// Optimal2x4p4o interpolates using optimal 2x (4-point, 4th-order) (z-form).
//
// The "2x" is a requirement, not a label: the coefficients minimize error over
// the lower half of the band, assuming the signal is oversampled by at least
// two. Fed a signal that uses its whole band, this is worse than the plainer
// Lagrange and Hermite interpolators above. Optimal2x6p5o is the same
// assumption over six points.
func Optimal2x4p4o[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	const middle = 1

	var s [4]T
	for i := range s {
		s[i] = sampleAt(src, xi-middle+i)
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	c0 := even1*0.45645918406487612 + even2*0.04354173901996461
	c1 := odd1*0.47236675362442071 + odd2*0.17686613581136501
	c2 := even1*-0.253674794204558521 + even2*0.25371918651882464
	c3 := odd1*-0.37917091811631082 + odd2*0.11952965967158000
	c4 := even1*0.04252164479749607 + even2*-0.04289144034653719

	z := frac - 1.0/2.0
	return (((c4*z+c3)*z+c2)*z+c1)*z + c0
}

// Optimal2x6p5o interpolates using optimal 2x (6-point, 5th-order) (z-form).
// It assumes a 2x oversampled input, as Optimal2x4p4o does; see the note there.
func Optimal2x6p5o[T Float](src []T, x T) T {
	xi, frac, ok := splitPos(len(src), x)
	if !ok {
		return 0
	}
	const middle = 2

	var s [6]T
	for i := range s {
		s[i] = sampleAt(src, xi-middle+i)
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	even3 := s[middle+3] + s[middle-2]
	odd3 := s[middle+3] - s[middle-2]
	c0 := even1*0.40513396007145713 + even2*0.09251794438424393 + even3*0.00234806603570670
	c1 := odd1*0.28342806338906690 + odd2*0.21703277024054901 + odd3*0.01309294748731515
	c2 := even1*-0.191337682540351941 + even2*0.16187844487943592 + even3*0.02946017143111912
	c3 := odd1*-0.16471626190554542 + odd2*-0.00154547203542499 + odd3*0.03399271444851909
	c4 := even1*0.03845798729588149 + even2*-0.05712936104242644 + even3*0.01866750929921070
	c5 := odd1*0.04317950185225609 + odd2*-0.01802814255926417 + odd3*0.00152170021558204

	z := frac - 1.0/2.0
	return ((((c5*z+c4)*z+c3)*z+c2)*z+c1)*z + c0
}
