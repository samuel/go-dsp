package dsp

import "samuelks.com/dsp/internal/f32"

// Rotator90 multiplies successive samples by successive powers of j
// (1, j, -1, -j), which shifts a complex signal by a quarter of the sample
// rate. A power of j only moves and negates the real and imaginary parts, so
// this costs no multiplies.
//
// It carries the phase across calls, which is why it is a type rather than a
// function: the sequence repeats every four samples, so a stream fed in blocks
// whose length is not a multiple of four has to resume where the last block
// left off.
type Rotator90[C Complex] struct {
	phase int
}

// NewRotator90 returns a rotator at the start of the sequence.
func NewRotator90[C Complex]() *Rotator90[C] {
	return &Rotator90[C]{}
}

// OutputLen returns inputs: the rotator writes one sample per input.
func (r *Rotator90[C]) OutputLen(inputs int) int { return inputs }

// Process writes the rotated src to dst, returns len(src) and advances the
// phase, so consecutive calls continue one stream whatever the block lengths
// are. It panics if dst is shorter than src. The slices may be the same.
func (r *Rotator90[C]) Process(dst, src []C) int {
	checkDst("Rotator90", len(dst), len(src))
	samples := dst[:len(src)]
	copy(samples, src)
	// The assembly only handles complex64 from the start of the sequence over
	// whole groups of four. Bring the phase back to zero with the generic loop,
	// hand the assembly every whole group that follows, and finish the tail the
	// same way.
	s, ok := any(samples).([]complex64)
	if !ok {
		r.phase = rotate90From(r.phase, samples)
		return len(src)
	}
	if r.phase != 0 {
		head := min(4-r.phase, len(s))
		r.phase = rotate90From(r.phase, s[:head])
		s = s[head:]
		if r.phase != 0 {
			return len(src)
		}
	}
	n := len(s) &^ 3
	f32.Rotate90(s[:n])
	r.phase = rotate90From(0, s[n:])
	return len(src)
}

// Reset restarts the sequence at 1, so the next block begins a new stream.
func (r *Rotator90[C]) Reset() {
	r.phase = 0
}

// rotate90From rotates samples starting at the given phase and returns the
// phase the next block starts from.
//
// It goes through complex128 because the real and imag builtins do not accept
// an argument of type-parameter type; see the note in cmplx.go. Swapping and
// negating the parts is exact, and so is the round trip through the wider type,
// so a complex64 result is the same value the concrete version produces.
func rotate90From[C Complex](phase int, samples []C) int {
	for i := range samples {
		switch (phase + i) & 3 {
		case 1:
			z := complex128(samples[i])
			samples[i] = C(complex(-imag(z), real(z)))
		case 2:
			samples[i] = -samples[i]
		case 3:
			z := complex128(samples[i])
			samples[i] = C(complex(imag(z), -real(z)))
		}
	}
	return (phase + len(samples)) & 3
}
