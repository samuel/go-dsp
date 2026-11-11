package f32

import (
	"testing"

	"samuelks.com/dsp/internal/kernelset"
)

// TestKernelSetComplete requires every build-tag group of each kernel family
// to declare the family's whole set of names with the same signatures.
func TestKernelSetComplete(t *testing.T) {
	for _, fam := range []struct {
		glob  string
		names []string
	}{
		// The per-width kernels behind dsp's generic vector functions.
		{"math32_*.go", []string{"Scale", "Max", "Min", "CAbs"}},
		// The FIR and dot-product leaves behind the checked wrappers in fir.go.
		{"fir_*.go", []string{"firF32", "cfirReal", "cfir", "dotF32", "cdotReal"}},
		// The per-sample kernels, assembly on arm and the references elsewhere.
		{"sample_*.go", []string{"FastAtan2", "FastAtan2Fine", "Rotate90", "FMDemod", "BoxcarDecimate", "RationalBoxcarDecimate"}},
	} {
		t.Run(fam.glob, func(t *testing.T) {
			kernelset.Check(t, fam.glob, fam.names)
		})
	}
}
