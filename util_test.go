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

func TestRotate90(t *testing.T) {
	src := make([]complex64, 256)
	for i := range 256 {
		src[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	dst := make([]complex64, 256)
	copy(dst, src)
	rotate90Asm(dst)
	expected := make([]complex64, 256)
	copy(expected, src)
	rotate90(expected)
	if len(dst) != len(expected) {
		t.Fatalf("Output doesn't match expected: %+v != %+v", dst, expected)
	}
	for i := range dst {
		if dst[i] != expected[i] {
			t.Fatalf("Output doesn't match expected: %+v != %+v", dst, expected)
		}
	}
}

func BenchmarkRotate90(b *testing.B) {
	src := make([]complex64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		rotate90Asm(src)
	}
}

func BenchmarkRotate90_Go(b *testing.B) {
	src := make([]complex64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		rotate90(src)
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
	NewRotator90[complex64]().Rotate(want)

	for _, block := range []int{1, 2, 3, 4, 5, 7, 8, 16, n - 1, n, n + 1} {
		t.Run(strconv.Itoa(block), func(t *testing.T) {
			got := slices.Clone(src)
			r := NewRotator90[complex64]()
			for i := 0; i < len(got); i += block {
				r.Rotate(got[i:min(i+block, len(got))])
			}
			if !slices.Equal(got, want) {
				t.Fatalf("in blocks of %d: %v, want %v", block, got, want)
			}
		})
	}

	// Reset restarts the sequence, so a rotator reused for a second stream
	// behaves like a fresh one.
	r := NewRotator90[complex64]()
	r.Rotate(make([]complex64, 3))
	r.Reset()
	got := slices.Clone(src)
	r.Rotate(got)
	if !slices.Equal(got, want) {
		t.Fatalf("after Reset: %v, want %v", got, want)
	}
}

// TestRotator90Widths checks the complex128 instantiation against the
// complex64 one. Only complex64 has assembly behind it, so this is what covers
// the generic loop at the other width.
func TestRotator90Widths(t *testing.T) {
	const n = 37
	c64 := make([]complex64, n)
	c128 := make([]complex128, n)
	for i := range c64 {
		c64[i] = complex(float32(i+1), float32(-(i + 1)))
		c128[i] = complex(float64(i+1), float64(-(i + 1)))
	}
	NewRotator90[complex64]().Rotate(c64)
	NewRotator90[complex128]().Rotate(c128)
	for i := range c64 {
		if complex128(c64[i]) != c128[i] {
			t.Fatalf("[%d]: complex64 %v, complex128 %v", i, c64[i], c128[i])
		}
	}
}

// TestRotator90FourfoldMatchesTheAssembly pins the fast path against the
// generic one: whole groups of four from phase zero are what Rotate hands to
// the assembly, and it has to agree with the loop that would otherwise have
// run.
func TestRotator90FourfoldMatchesTheAssembly(t *testing.T) {
	for _, n := range []int{0, 4, 8, 64, 256} {
		src := make([]complex64, n)
		for i := range src {
			src[i] = complex(float32(i)-128, -(float32(i) - 128))
		}
		viaAsm := slices.Clone(src)
		NewRotator90[complex64]().Rotate(viaAsm)
		viaGo := slices.Clone(src)
		rotate90From(0, viaGo)
		if !slices.Equal(viaAsm, viaGo) {
			t.Fatalf("n=%d: assembly %v, generic %v", n, viaAsm, viaGo)
		}
	}
}

// TestRotate90Ragged checks that a length which is not a whole number of
// groups leaves the tail alone instead of running off the end. Both filters
// used to step past it, and did so after having already rewritten part of the
// caller's buffer in place.
func TestRotate90Ragged(t *testing.T) {
	for n := range 13 {
		src := make([]complex64, n)
		for i := range src {
			src[i] = complex(float32(i+1), float32(-(i + 1)))
		}
		asm := slices.Clone(src)
		ref := slices.Clone(src)
		rotate90Asm(asm)
		rotate90(ref)
		got, want := asm, ref
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("n=%d [%d] = %v, want %v", n, i, got[i], want[i])
			}
		}
		// Everything past the last whole group has to be untouched.
		for i := n &^ 3; i < n; i++ {
			if got[i] != src[i] {
				t.Fatalf("n=%d: rotated the partial tail at %d: %v, was %v", n, i, got[i], src[i])
			}
		}
	}
}
