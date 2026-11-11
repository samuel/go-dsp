package dsp

import (
	"math"
	"slices"
	"strconv"
	"testing"
)

func approxEqual(a, b, e float64) bool {
	return math.Abs(a-b) <= e
}

func BenchmarkRotate90(b *testing.B) {
	src := make([]complex64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	r := NewRotator90[complex64]()
	for range b.N {
		r.Process(src, src)
	}
}

func BenchmarkRotate90Generic(b *testing.B) {
	src := make([]complex64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		rotate90From(0, src)
	}
}

// TestRotator90CarriesThePhase checks that a stream fed in blocks comes out
// the same however it is cut up.
//
// The point of the type is that the sequence 1, j, -1, -j continues
// across calls: rotating in blocks of three used to restart it at 1 every time,
// so two thirds of every block after the first was multiplied by the wrong
// power of j -- a quarter-rate shift that silently was not one.
func TestRotator90CarriesThePhase(t *testing.T) {
	const n = 61
	src := make([]complex64, n)
	for i := range src {
		src[i] = complex(float32(i+1), float32(-(i + 1)))
	}

	want := slices.Clone(src)
	NewRotator90[complex64]().Process(want, want)

	for _, block := range []int{1, 2, 3, 4, 5, 7, 8, 16, n - 1, n, n + 1} {
		t.Run(strconv.Itoa(block), func(t *testing.T) {
			got := slices.Clone(src)
			r := NewRotator90[complex64]()
			for i := 0; i < len(got); i += block {
				blk := got[i:min(i+block, len(got))]
				r.Process(blk, blk)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("in blocks of %d: %v, want %v", block, got, want)
			}
		})
	}

	// Reset restarts the sequence, so a rotator reused for a second stream
	// behaves like a fresh one.
	r := NewRotator90[complex64]()
	r.Process(make([]complex64, 3), make([]complex64, 3))
	r.Reset()
	got := slices.Clone(src)
	r.Process(got, got)
	if !slices.Equal(got, want) {
		t.Fatalf("after Reset: %v, want %v", got, want)
	}
}

// TestRotator90Widths checks the complex128 instantiation against the
// complex64 one. Only complex64 has a kernel behind it, so this is what covers
// the generic loop at the other width.
func TestRotator90Widths(t *testing.T) {
	const n = 37
	c64 := make([]complex64, n)
	c128 := make([]complex128, n)
	for i := range c64 {
		c64[i] = complex(float32(i+1), float32(-(i + 1)))
		c128[i] = complex(float64(i+1), float64(-(i + 1)))
	}
	NewRotator90[complex64]().Process(c64, c64)
	NewRotator90[complex128]().Process(c128, c128)
	for i := range c64 {
		if complex128(c64[i]) != c128[i] {
			t.Fatalf("[%d]: complex64 %v, complex128 %v", i, c64[i], c128[i])
		}
	}
}

// TestRotator90FourfoldMatchesTheAssembly pins the fast path against the
// generic one: whole groups of four from phase zero are what Process hands to
// the kernel, and it has to agree with the loop that would otherwise have run.
func TestRotator90FourfoldMatchesTheAssembly(t *testing.T) {
	for _, n := range []int{0, 4, 8, 64, 256} {
		src := make([]complex64, n)
		for i := range src {
			src[i] = complex(float32(i)-128, -(float32(i) - 128))
		}
		viaKernel := slices.Clone(src)
		NewRotator90[complex64]().Process(viaKernel, viaKernel)
		viaGo := slices.Clone(src)
		rotate90From(0, viaGo)
		if !slices.Equal(viaKernel, viaGo) {
			t.Fatalf("n=%d: kernel %v, generic %v", n, viaKernel, viaGo)
		}
	}
}
