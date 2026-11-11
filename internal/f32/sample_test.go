package f32

import (
	"math"
	"math/rand"
	"slices"
	"testing"
)

// atan2Sweep is the argument sweep for the exact comparisons: both axes, both
// diagonals, both zeros, the rails and the non-finite cases, plus enough
// ordinary values to reach every quadrant and both sides of FastAtan2Fine's
// |y/x| == 1 branch.
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

// sameFloat32 compares two results exactly, treating any two NaNs as equal --
// the sign and payload of a NaN are not part of any contract here -- and
// distinguishing +0 from -0, which is.
func sameFloat32(a, b float32) bool {
	if a != a && b != b {
		return true
	}
	return math.Float32bits(a) == math.Float32bits(b)
}

// TestFastAtan2MatchesReference compares the kernels with the references bit
// for bit rather than within a tolerance.
//
// A tolerance is the wrong comparison here: the reference is the definition of
// the function, and the interesting disagreements are exactly the ones a
// tolerance hides -- a branch taken on >= instead of >, or a negative zero
// where the reference returns +0 because the ARM condition LT is also true for
// an unordered compare. All of them are within any tolerance a caller would
// pick.
func TestFastAtan2MatchesReference(t *testing.T) {
	vs := atan2Sweep()
	for _, p := range []struct {
		name     string
		got, ref func(y, x float32) float32
	}{
		{"FastAtan2", FastAtan2, fastAtan2},
		{"FastAtan2Fine", FastAtan2Fine, fastAtan2Fine},
	} {
		t.Run(p.name, func(t *testing.T) {
			for _, y := range vs {
				for _, x := range vs {
					if got, want := p.got(y, x), p.ref(y, x); !sameFloat32(got, want) {
						t.Fatalf("%s(%v, %v) = %v, reference gives %v", p.name, y, x, got, want)
					}
				}
			}
		})
	}
}

// TestFMDemodMatchesReference is the same comparison for the demodulator,
// whose arm implementation inlines its own copy of FastAtan2 and so has to
// agree on the same edges, and carry the same sample out.
func TestFMDemodMatchesReference(t *testing.T) {
	vs := atan2Sweep()
	var src []complex64
	for _, re := range vs {
		for _, im := range vs {
			src = append(src, complex(re, im))
		}
	}
	pre := complex64(complex(0.25, -3))
	got := make([]float32, len(src))
	want := make([]float32, len(src))
	gotPre := FMDemod(got, src, pre)
	wantPre := fmDemod(want, src, pre)
	for i := range got {
		if !sameFloat32(got[i], want[i]) {
			t.Fatalf("sample %d (%v): got %v, reference gives %v", i, src[i], got[i], want[i])
		}
	}
	if !sameFloat32(real(gotPre), real(wantPre)) || !sameFloat32(imag(gotPre), imag(wantPre)) {
		t.Fatalf("carried sample: got %v, reference gives %v", gotPre, wantPre)
	}
}

// TestFMDemodShortOutput checks that the kernel processes
// min(len(dst), len(src)) samples, never writes past dst, and returns pre
// untouched when it reads nothing.
func TestFMDemodShortOutput(t *testing.T) {
	src := []complex64{complex(0, 2), complex(1, 2), complex(-3, 7), complex(4, -9), complex(1, 1)}
	pre := complex64(complex(2, 1))
	for n := range len(src) + 2 {
		dst := make([]float32, n+1)
		gotPre := FMDemod(dst[:n], src, pre)
		if dst[n] != 0 {
			t.Fatalf("n=%d: wrote past the end: %v", n, dst)
		}
		ref := make([]float32, n)
		wantPre := fmDemod(ref, src, pre)
		for i := range ref {
			if !sameFloat32(dst[i], ref[i]) {
				t.Fatalf("n=%d [%d]: %v, reference %v", n, i, dst[i], ref[i])
			}
		}
		if gotPre != wantPre {
			t.Fatalf("n=%d: carried %v, reference %v", n, gotPre, wantPre)
		}
	}
}

// TestRotate90MatchesReference checks every length up to 13, so that a
// partial trailing group is left untouched rather than rotated or overrun.
func TestRotate90MatchesReference(t *testing.T) {
	for n := range 14 {
		src := make([]complex64, n+1)
		for i := range src {
			src[i] = complex(float32(i+1), float32(-(i + 1)))
		}
		got := slices.Clone(src)
		want := slices.Clone(src)
		Rotate90(got[:n])
		rotate90(want[:n])
		if !slices.Equal(got, want) {
			t.Fatalf("n=%d: %v, reference %v", n, got, want)
		}
		for i := n &^ 3; i <= n; i++ {
			if got[i] != src[i] {
				t.Fatalf("n=%d: touched %d past the last whole group", n, i)
			}
		}
	}
}

// TestBoxcarDecimateMatchesReference feeds a stream in uneven blocks and
// carries the partial group across them, as the stateful type does.
func TestBoxcarDecimateMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	src := randC64(r, 300)
	for _, factor := range []int{1, 2, 3, 7} {
		var gotOut, wantOut []complex64
		var gs, ws complex64
		var gc, wc int
		for i, block := 0, 1; i < len(src); i, block = i+block, block%11+1 {
			in := src[i:min(i+block, len(src))]
			g := make([]complex64, (gc+len(in))/factor+1)
			w := make([]complex64, len(g))
			var gn, wn int
			gn, gs, gc = BoxcarDecimate(g, in, factor, gs, gc)
			wn, ws, wc = boxcarDecimate(w, in, factor, ws, wc)
			if g[len(g)-1] != 0 {
				t.Fatalf("factor %d: wrote past the outputs", factor)
			}
			gotOut = append(gotOut, g[:gn]...)
			wantOut = append(wantOut, w[:wn]...)
		}
		if !slices.Equal(gotOut, wantOut) || gs != ws || gc != wc {
			t.Fatalf("factor %d: %v (%v, %d), reference %v (%v, %d)", factor, gotOut, gs, gc, wantOut, ws, wc)
		}
	}
}

func TestRationalBoxcarDecimateMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(10))
	src := randF32(r, 300)
	for _, rates := range [][2]int{{3, 2}, {48, 11}, {5, 5}, {7, 1}} {
		fast, slow := rates[0], rates[1]
		var gotOut, wantOut []float32
		var gs, ws float32
		var gc, wc, gp, wp int
		for i, block := 0, 1; i < len(src); i, block = i+block, block%13+1 {
			in := src[i:min(i+block, len(src))]
			g := make([]float32, (gp+len(in)*slow)/fast+1)
			w := make([]float32, len(g))
			var gn, wn int
			gn, gs, gc, gp = RationalBoxcarDecimate(g, in, fast, slow, gs, gc, gp)
			wn, ws, wc, wp = rationalBoxcarDecimate(w, in, fast, slow, ws, wc, wp)
			if g[len(g)-1] != 0 {
				t.Fatalf("%d/%d: wrote past the outputs", fast, slow)
			}
			gotOut = append(gotOut, g[:gn]...)
			wantOut = append(wantOut, w[:wn]...)
		}
		if !slices.Equal(gotOut, wantOut) || gs != ws || gc != wc || gp != wp {
			t.Fatalf("%d/%d: %v (%v, %d, %d), reference %v (%v, %d, %d)",
				fast, slow, gotOut, gs, gc, gp, wantOut, ws, wc, wp)
		}
	}
}

func BenchmarkFMDemod(b *testing.B) {
	r := rand.New(rand.NewSource(11))
	src := randC64(r, 4096)
	dst := make([]float32, len(src))
	b.SetBytes(int64(len(src)) * 8)
	b.Run("kernel", func(b *testing.B) {
		for b.Loop() {
			FMDemod(dst, src, 0)
		}
	})
	b.Run("reference", func(b *testing.B) {
		for b.Loop() {
			fmDemod(dst, src, 0)
		}
	})
}

func BenchmarkFastAtan2(b *testing.B) {
	r := rand.New(rand.NewSource(12))
	xy := randF32(r, 2048)
	b.Run("kernel", func(b *testing.B) {
		for b.Loop() {
			for i := 0; i < len(xy); i += 2 {
				FastAtan2(xy[i], xy[i+1])
			}
		}
	})
	b.Run("reference", func(b *testing.B) {
		for b.Loop() {
			for i := 0; i < len(xy); i += 2 {
				fastAtan2(xy[i], xy[i+1])
			}
		}
	})
}
