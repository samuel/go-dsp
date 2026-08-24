package dsp

import (
	"math"
	"testing"
)

// atan2Sweep is the argument sweep for the exact assembly-versus-reference
// comparisons: both axes, both diagonals, both zeros, the rails and the
// non-finite cases, plus enough ordinary values to reach every quadrant and
// both sides of FastAtan2Fine's |y/x| == 1 branch.
func atan2Sweep() []float32 {
	vs := []float32{
		0, float32(math.Copysign(0, -1)),
		1, -1, 0.5, -0.5, 2, -2,
		1e-20, -1e-20, 1e20, -1e20,
		math.MaxFloat32, -math.MaxFloat32, math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	}
	for i := -13; i <= 13; i++ {
		vs = append(vs, float32(i)*0.37)
	}
	return vs
}

// TestFastAtan2MatchesReference compares the assembly against the Go reference
// bit for bit rather than within a tolerance.
//
// A tolerance is the wrong comparison here: the reference is the definition of
// the function, and the interesting disagreements are exactly the ones a
// tolerance hides. The arm code returned 0.78125 where the reference returns
// 0.78960 for FastAtan2Fine(1, 1) -- one branch taken on >= instead of > --
// and it handed back a negative zero, or -Pi/2 for a NaN, where the reference
// returns +0, because the ARM condition LT is also true for an unordered
// compare. All four are within any tolerance a caller would pick.
func TestFastAtan2MatchesReference(t *testing.T) {
	vs := atan2Sweep()
	pairs := []struct {
		name string
		asm  func(y, x float32) float32
		ref  func(y, x float32) float32
	}{
		{"FastAtan2", fastAtan2Asm, fastAtan2},
		{"FastAtan2Fine", fastAtan2FineAsm, fastAtan2Fine},
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			for _, y := range vs {
				for _, x := range vs {
					got, want := p.asm(y, x), p.ref(y, x)
					if !sameFloat32(got, want) {
						t.Fatalf("%s(%v, %v) = %v, reference gives %v", p.name, y, x, got, want)
					}
				}
			}
		})
	}
}

// TestFMDemodulateMatchesReference is the same comparison for the demodulator,
// whose arm implementation inlines its own copy of FastAtan2 and so has to
// agree on the same edges.
func TestFMDemodulateMatchesReference(t *testing.T) {
	vs := atan2Sweep()
	var src []complex64
	for _, re := range vs {
		for _, im := range vs {
			src = append(src, complex(re, im))
		}
	}

	got := make([]float32, len(src))
	want := make([]float32, len(src))
	var a, b FMDemod
	fmDemodulateAsm(&a, got, src)
	fmDemodulate(&b, want, src)
	for i := range got {
		if !sameFloat32(got[i], want[i]) {
			t.Fatalf("sample %d (%v after %v): got %v, reference gives %v",
				i, src[i], preOf(src, i), got[i], want[i])
		}
	}
	if !sameFloat32(real(a.pre), real(b.pre)) || !sameFloat32(imag(a.pre), imag(b.pre)) {
		t.Fatalf("carried sample: got %v, reference gives %v", a.pre, b.pre)
	}
}

func preOf(src []complex64, i int) complex64 {
	if i == 0 {
		return 0
	}
	return src[i-1]
}

// sameFloat32 compares two results exactly, treating any two NaNs as equal --
// the sign and payload of a NaN are not part of any contract here -- and
// distinguishing +0 from -0, which is.
func sameFloat32(a, b float32) bool {
	if a != a && b != b {
		return true
	}
	return math.Float32bits(a) == math.Float32bits(b)
}
