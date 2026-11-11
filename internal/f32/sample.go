package f32

import "math"

// The per-sample kernels: each output depends on the one before it, through
// a carried sample, a running sum or a branch per sample, so they do not
// vectorize. 32-bit arm has VFP assembly for them in sample_arm.s; every other
// architecture runs the references below, which define their behavior.
//
// They hold no state. The stateful types in dsp pass theirs in and store what
// comes back.

const (
	pi2  = math.Pi / 2
	pi4  = math.Pi / 4
	pi34 = math.Pi * 3 / 4
)

// fastAtan2 is the reference for FastAtan2.
func fastAtan2(y, x float32) float32 {
	absY := max(y, -y)
	absY += 1e-20 // kludge to prevent 0/0 condition
	var angle float32
	switch {
	case x < 0.0:
		r := (x + absY) / (absY - x)
		angle = pi34 + float32(quadPoly(r)*r)
	case x > 0.0:
		r := (x - absY) / (x + absY)
		angle = pi4 + float32(quadPoly(r)*r)
	case y < 0.0:
		return -pi2
	case y > 0.0:
		return pi2
	default:
		return 0.0
	}
	if y < 0.0 {
		return -angle // negate if in quad III or IV
	}
	return angle
}

// quadPoly is 0.1963*r*r - 0.9817, the odd part of FastAtan2's quadrant
// polynomial, with the product rounded before the subtraction.
//
// The explicit conversion is needed to ensure that arm64 does not change
// the multiply and the subtraction into one FMSUB, which the unfused
// MULF/SUBF pair in sample_arm.s cannot reproduce, and the two architectures
// then disagree on the low bit of most inputs. Its caller rounds
// the outer product for the same reason -- FMADD with pi34 or pi4.
func quadPoly(r float32) float32 { return float32(0.1963*r*r) - 0.9817 }

// fastAtan2Fine is the reference for FastAtan2Fine.
func fastAtan2Fine(y, x float32) float32 {
	if x == 0.0 {
		switch {
		case y > 0.0:
			return pi2
		case y < 0.0:
			return -pi2
		}
		return 0.0
	}
	z := y / x
	// Rounded explicitly, so that z*z cannot contract with the + 0.28 below
	// into an FMADD that sample_arm.s's separate MULF and ADDF do not produce.
	zz := float32(z * z)
	if zz < 1.0 {
		atan := z / (1.0 + float32(0.28*zz))
		if x < 0.0 {
			if y < 0.0 {
				return atan - math.Pi
			}
			return atan + math.Pi
		}
		return atan
	}
	atan := pi2 - z/(zz+0.28)
	if y < 0.0 {
		return atan - math.Pi
	}
	return atan
}

// polarDiscriminator is fastAtan2 of a*conj(b). Each product is rounded before
// it is added or subtracted. Without that, arm64 contracts the two pairs into
// FMSUB and FMADD, which the four separate MULF instructions in sample_arm.s
// cannot reproduce; see the note on quadPoly.
func polarDiscriminator(a, b complex64) float32 {
	return fastAtan2(
		float32(imag(a)*real(b))-float32(real(a)*imag(b)),
		float32(real(a)*real(b))+float32(imag(a)*imag(b)),
	)
}

// rotate90 is the reference for Rotate90.
func rotate90(samples []complex64) {
	n := len(samples) &^ 3
	for i := 0; i < n; i += 4 {
		samples[i+1] = complex(-imag(samples[i+1]), real(samples[i+1]))
		samples[i+2] = -samples[i+2]
		samples[i+3] = complex(imag(samples[i+3]), -real(samples[i+3]))
	}
}

// fmDemod is the reference for FMDemod.
func fmDemod(dst []float32, src []complex64, pre complex64) complex64 {
	n := min(len(src), len(dst))
	for i, inp := range src[:n] {
		dst[i] = polarDiscriminator(inp, pre)
		pre = inp
	}
	return pre
}

// boxcarDecimate is the reference for BoxcarDecimate.
func boxcarDecimate(dst, src []complex64, factor int, sum complex64, count int) (int, complex64, int) {
	n := 0
	for _, v := range src {
		sum += v
		count++
		if count < factor {
			continue
		}
		dst[n] = sum
		n++
		count = 0
		sum = 0
	}
	return n, sum, count
}

// rationalBoxcarDecimate is the reference for RationalBoxcarDecimate.
func rationalBoxcarDecimate(dst, src []float32, fast, slow int, sum float32, count, phase int) (int, float32, int, int) {
	n := 0
	for _, v := range src {
		sum += v
		count++
		phase += slow
		if phase < fast {
			continue
		}
		// Divide by the count this output actually accumulated, not by the
		// nominal fast/slow. For a non-integer ratio the count alternates
		// between the floor and the ceiling of it, and a fixed divisor turns
		// that alternation into a spurious tone at the beat frequency.
		dst[n] = sum / float32(count)
		n++
		phase -= fast
		sum = 0
		count = 0
	}
	return n, sum, count, phase
}
