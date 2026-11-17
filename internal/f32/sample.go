package f32

import "math"

// The per-sample kernels: each output depends on the one before it, through
// a carried sample, a running sum or a branch per sample, so they do not
// vectorize. 32-bit arm has VFP assembly for them in sample_arm.s; every other
// architecture runs the references below, which define their behavior.
//
// They hold no state. The stateful types in dsp pass theirs in and store what
// comes back.

const pi2 = math.Pi / 2

// The coefficients of atan(z) ~ z*(atanC1 + atanC3*z^2 + atanC5*z^4 +
// atanC7*z^6) on [0, 1], fitted by Remez exchange to the least maximum
// relative error, 2.1e-4. Relative rather than absolute error keeps the slope
// at zero within that bound, so an FM discriminator's small phase steps come
// out at the right scale; the absolute error is at most 1.7e-4 radians.
const (
	atanC1 = 0.999787867
	atanC3 = -0.325808436
	atanC5 = 0.155578747
	atanC7 = -0.0443266146
)

// fastAtan2 is the reference for FastAtan2. It folds (x, y) into the first
// octant, where the smaller magnitude over the larger is in [0, 1], and
// unfolds the polynomial's angle from there.
func fastAtan2(y, x float32) float32 {
	ax, ay := abs(x), abs(y)
	var a float32
	switch {
	case ay > ax:
		a = pi2 - atanPoly(ax/ay)
	case ax > 0:
		a = atanPoly(ay / ax)
	default:
		// Both zero, or a NaN in either: +0 or NaN.
		return ax + ay
	}
	if x < 0 {
		a = math.Pi - a
	}
	if y < 0 {
		a = -a
	}
	return a
}

// atanPoly evaluates the polynomial for z in [0, 1]. Each product is rounded
// before it is added so that arm64 cannot fuse an FMADD that the separate
// MULF and ADDF in sample_arm.s do not produce.
func atanPoly(z float32) float32 {
	s := float32(z * z)
	p := float32(atanC7*s) + atanC5
	p = float32(p*s) + atanC3
	p = float32(p*s) + atanC1
	return float32(p * z)
}

func abs(v float32) float32 { return math.Float32frombits(math.Float32bits(v) &^ (1 << 31)) }

// polarDiscriminator is fastAtan2 of a*conj(b). Each product is rounded before
// it is added or subtracted. Without that, arm64 contracts the two pairs into
// FMSUB and FMADD, which the four separate MULF instructions in sample_arm.s
// cannot reproduce.
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
