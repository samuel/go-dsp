package dspviz

import "math"

// Tone fills dst with amp*cos(2*pi*k*i/period), a cosine of exactly k cycles
// in every period samples, and returns it.
//
// The phase is formed as 2*pi*((k*i) mod period)/period, in integers, not as
// 2*pi*freq*i/rate: that argument grows with i -- to 3e6 radians over a 1M
// sample frame -- and float64's spacing then eats the low bits of every angle
// in a pattern that is not white, so the result is a floor of spurious tones
// rather than rounding noise. Measured, that costs between 20 and 115 dB of
// headroom, more on longer frames and higher tones; the integer form costs
// nothing and keeps it.
//
// A tone read back over a whole number of periods leaks nothing into any other
// bin, which is what lets a stopband be measured through a rectangular window,
// far below whatever its sidelobes would allow.
func Tone(dst []float64, k, period int, amp float64) []float64 {
	if period <= 0 {
		panic("dspviz: tone period must be positive")
	}
	for i := range dst {
		dst[i] = amp * math.Cos(2*math.Pi*float64(int64(k)*int64(i)%int64(period))/float64(period))
	}
	return dst
}

// CoherentFreq returns the frequency nearest freq that completes a whole number
// of cycles in a frame of n samples at rate, along with that cycle count, the
// bin the tone will land in.
//
// The count is forced odd: n is generally a power of two, so an odd count is
// coprime to it and no harmonic can fold back onto the tone's own bin, which is
// what lets the harmonics be measured separately from the fundamental. The
// result is clamped away from DC and Nyquist.
func CoherentFreq(freq, rate float64, n int) (hz float64, cycles int) {
	if n <= 0 {
		panic("dspviz: frame must be positive")
	}
	k := int(math.Round(freq * float64(n) / rate))
	if k&1 == 0 {
		// Round to the nearer odd neighbor rather than always upward, so the
		// requested frequency is not biased in one direction.
		if freq*float64(n)/rate > float64(k) {
			k++
		} else {
			k--
		}
	}
	k = max(1, min(k, n/2-1|1))
	return float64(k) * rate / float64(n), k
}
