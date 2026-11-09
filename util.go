package dsp

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

// Rotate rotates samples in place and advances the phase, so consecutive calls
// continue one stream whatever the block lengths are.
func (r *Rotator90[C]) Rotate(samples []C) {
	// The assembly only handles complex64 from the start of the sequence over
	// whole groups of four. Bring the phase back to zero with the generic loop,
	// hand the assembly every whole group that follows, and finish the tail the
	// same way.
	s, ok := any(samples).([]complex64)
	if !ok {
		r.phase = rotate90From(r.phase, samples)
		return
	}
	if r.phase != 0 {
		head := min(4-r.phase, len(s))
		r.phase = rotate90From(r.phase, s[:head])
		s = s[head:]
		if r.phase != 0 {
			return
		}
	}
	n := len(s) &^ 3
	rotate90Asm(s[:n])
	r.phase = rotate90From(0, s[n:])
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

// rotate90 is the complex64 rotation from phase zero over whole groups of four,
// the pattern the assembly implements and the Go reference the stubs on
// architectures without their own version jump to. A trailing group of fewer
// than four is left untouched, which is why callers hand it a length that is a
// multiple of four and rotate the remainder themselves.
func rotate90(samples []complex64) {
	n := len(samples) &^ 3
	for i := 0; i < n; i += 4 {
		samples[i+1] = complex(-imag(samples[i+1]), real(samples[i+1]))
		samples[i+2] = -samples[i+2]
		samples[i+3] = complex(imag(samples[i+3]), -real(samples[i+3]))
	}
}
