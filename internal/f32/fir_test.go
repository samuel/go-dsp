package f32

import (
	"math"
	"math/rand"
	"strconv"
	"testing"
)

// firTapCounts covers one tap, the counts either side of every vector width
// and unroll a kernel might use, and a long filter.
var firTapCounts = []int{1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 32, 33, 64}

// randF32 returns n values in [-1, 1), none of them subnormal. 32-bit arm's
// NEON flushes subnormals to zero where the references do not, so they are
// kept out of the comparison.
func randF32(r *rand.Rand, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = 2*r.Float32() - 1
	}
	return out
}

func randC64(r *rand.Rand, n int) []complex64 {
	out := make([]complex64, n)
	for i := range out {
		out[i] = complex(2*r.Float32()-1, 2*r.Float32()-1)
	}
	return out
}

func sameBits(a, b float32) bool { return math.Float32bits(a) == math.Float32bits(b) }

func sameC64(a, b complex64) bool { return sameBits(real(a), real(b)) && sameBits(imag(a), imag(b)) }

// firWorkLen is how much work a call with n outputs reads.
func firWorkLen(n, taps, stride int) int {
	if n == 0 {
		return taps - 1
	}
	return (n-1)*stride + taps
}

// TestFIRMatchesReference compares every FIR kernel with its reference bit for
// bit, over every output count up to 67 and a few past it, every stride up to
// 3, and work that starts off any alignment. It also checks that nothing is
// written past dst.
func TestFIRMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	const guard = 4
	counts := make([]int, 0, 72)
	for n := range 68 {
		counts = append(counts, n)
	}
	counts = append(counts, 100, 127, 200)
	for _, taps := range firTapCounts {
		for stride := 1; stride <= 3; stride++ {
			t.Run(strconv.Itoa(taps)+"x"+strconv.Itoa(stride), func(t *testing.T) {
				h := randF32(r, taps)
				hc := randC64(r, taps)
				for _, n := range counts {
					off := n & 3
					wl := firWorkLen(n, taps, stride) + off
					w := randF32(r, wl)[off:]
					wc := randC64(r, wl)[off:]

					got := make([]float32, n+guard)
					want := make([]float32, n)
					FIR(got[:n], w, h, stride)
					firF32Go(want, w, h, stride)
					for k := range want {
						if !sameBits(got[k], want[k]) {
							t.Fatalf("FIR n=%d [%d] = %v, reference %v", n, k, got[k], want[k])
						}
					}
					for k := n; k < n+guard; k++ {
						if got[k] != 0 {
							t.Fatalf("FIR n=%d wrote past dst at %d", n, k)
						}
					}

					gotC := make([]complex64, n+guard)
					wantC := make([]complex64, n)
					CFIRReal(gotC[:n], wc, h, stride)
					cfirRealGo(wantC, wc, h, stride)
					for k := range wantC {
						if !sameC64(gotC[k], wantC[k]) {
							t.Fatalf("CFIRReal n=%d [%d] = %v, reference %v", n, k, gotC[k], wantC[k])
						}
					}
					for k := n; k < n+guard; k++ {
						if gotC[k] != 0 {
							t.Fatalf("CFIRReal n=%d wrote past dst at %d", n, k)
						}
					}

					clear(gotC)
					CFIR(gotC[:n], wc, hc, stride)
					cfirGo(wantC, wc, hc, stride)
					for k := range wantC {
						if !sameC64(gotC[k], wantC[k]) {
							t.Fatalf("CFIR n=%d [%d] = %v, reference %v", n, k, gotC[k], wantC[k])
						}
					}
					for k := n; k < n+guard; k++ {
						if gotC[k] != 0 {
							t.Fatalf("CFIR n=%d wrote past dst at %d", n, k)
						}
					}
				}
			})
		}
	}
}

// TestCFIRRealIsFIRPerPart checks the definition of real taps over a complex
// signal: the real filter run over each part.
func TestCFIRRealIsFIRPerPart(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for _, taps := range firTapCounts {
		h := randF32(r, taps)
		const n = 53
		w := randC64(r, firWorkLen(n, taps, 1))
		re := make([]float32, len(w))
		im := make([]float32, len(w))
		for i, v := range w {
			re[i], im[i] = real(v), imag(v)
		}
		got := make([]complex64, n)
		wantRe := make([]float32, n)
		wantIm := make([]float32, n)
		CFIRReal(got, w, h, 1)
		FIR(wantRe, re, h, 1)
		FIR(wantIm, im, h, 1)
		for k := range got {
			if !sameBits(real(got[k]), wantRe[k]) || !sameBits(imag(got[k]), wantIm[k]) {
				t.Fatalf("taps=%d [%d] = %v, want (%v, %v)", taps, k, got[k], wantRe[k], wantIm[k])
			}
		}
	}
}

// TestFIRReferenceIsAConvolution checks the references themselves against the
// convolution computed in float64, so an index slip in the definition cannot
// hide behind kernels that copy it.
func TestFIRReferenceIsAConvolution(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	const n, taps, stride = 20, 7, 2
	h := randF32(r, taps)
	hc := randC64(r, taps)
	w := randF32(r, firWorkLen(n, taps, stride))
	wc := randC64(r, firWorkLen(n, taps, stride))
	got := make([]float32, n)
	gotC := make([]complex64, n)
	gotCC := make([]complex64, n)
	firF32Go(got, w, h, stride)
	cfirRealGo(gotC, wc, h, stride)
	cfirGo(gotCC, wc, hc, stride)
	for k := range n {
		at := k*stride + taps - 1
		var want float64
		var wantC, wantCC complex128
		for j := range taps {
			want += float64(h[j]) * float64(w[at-j])
			wantC += complex(float64(h[j]), 0) * complex128(wc[at-j])
			wantCC += complex128(hc[j]) * complex128(wc[at-j])
		}
		if math.Abs(float64(got[k])-want) > 1e-5 {
			t.Errorf("FIR [%d] = %v, want %v", k, got[k], want)
		}
		if d := complex128(gotC[k]) - wantC; math.Hypot(real(d), imag(d)) > 1e-5 {
			t.Errorf("CFIRReal [%d] = %v, want %v", k, gotC[k], wantC)
		}
		if d := complex128(gotCC[k]) - wantCC; math.Hypot(real(d), imag(d)) > 1e-5 {
			t.Errorf("CFIR [%d] = %v, want %v", k, gotCC[k], wantCC)
		}
	}
}

func TestFIRChecks(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		dst, work, taps, stride int
	}{
		{"no taps", 1, 4, 0, 1},
		{"zero stride", 1, 4, 2, 0},
		{"short work", 3, 5, 4, 1},
		{"short work, strided", 3, 8, 4, 3},
		{"work shorter than taps", 1, 2, 3, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("no panic")
				}
			}()
			FIR(make([]float32, tc.dst), make([]float32, tc.work), make([]float32, tc.taps), tc.stride)
		})
	}
	// Exactly enough work is fine, and no outputs need no work past the history.
	FIR(make([]float32, 3), make([]float32, 10), make([]float32, 4), 3)
	FIR(nil, make([]float32, 3), make([]float32, 4), 1)
}

// TestDotMatchesReference compares Dot and CDotReal with their references bit
// for bit at every length up to 200, off every alignment.
func TestDotMatchesReference(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for n := range 201 {
		off := n & 3
		x := randF32(r, n+off+3)[off:]
		c := randF32(r, n)
		if got, want := Dot(x, c), dotF32Go(x[:n], c); !sameBits(got, want) {
			t.Fatalf("Dot n=%d = %v, reference %v", n, got, want)
		}
		xc := randC64(r, n+off+3)[off:]
		if got, want := CDotReal(xc, c), cdotRealGo(xc[:n], c); !sameC64(got, want) {
			t.Fatalf("CDotReal n=%d = %v, reference %v", n, got, want)
		}
	}
}

// TestCDotRealIsDotPerPart checks that the complex dot product is Dot of each
// part exactly, which is what lets one interpolator table read a complex
// signal as two real ones.
func TestCDotRealIsDotPerPart(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for n := range 70 {
		x := randC64(r, n)
		c := randF32(r, n)
		re := make([]float32, n)
		im := make([]float32, n)
		for i, v := range x {
			re[i], im[i] = real(v), imag(v)
		}
		got := CDotReal(x, c)
		if !sameBits(real(got), Dot(re, c)) || !sameBits(imag(got), Dot(im, c)) {
			t.Fatalf("n=%d: %v, want (%v, %v)", n, got, Dot(re, c), Dot(im, c))
		}
	}
}

func TestDotReferenceIsADotProduct(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	for _, n := range []int{0, 1, 7, 8, 9, 64, 101} {
		x := randF32(r, n)
		c := randF32(r, n)
		var want float64
		for i := range c {
			want += float64(x[i]) * float64(c[i])
		}
		if got := dotF32Go(x, c); math.Abs(float64(got)-want) > 1e-5 {
			t.Errorf("n=%d: %v, want %v", n, got, want)
		}
	}
}

func TestDotChecks(t *testing.T) {
	for _, fn := range []func(){
		func() { Dot(make([]float32, 2), make([]float32, 3)) },
		func() { CDotReal(make([]complex64, 2), make([]float32, 3)) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("no panic")
				}
			}()
			fn()
		}()
	}
}

func BenchmarkFIR(b *testing.B) {
	r := rand.New(rand.NewSource(7))
	const n = 1024
	for _, taps := range []int{8, 32, 128} {
		h := randF32(r, taps)
		hc := randC64(r, taps)
		w := randF32(r, n+taps-1)
		wc := randC64(r, n+taps-1)
		dst := make([]float32, n)
		dstC := make([]complex64, n)
		name := "taps=" + strconv.Itoa(taps)
		b.Run("FIR/"+name, func(b *testing.B) {
			b.SetBytes(n * 4)
			for b.Loop() {
				FIR(dst, w, h, 1)
			}
		})
		b.Run("CFIRReal/"+name, func(b *testing.B) {
			b.SetBytes(n * 8)
			for b.Loop() {
				CFIRReal(dstC, wc, h, 1)
			}
		})
		b.Run("CFIR/"+name, func(b *testing.B) {
			b.SetBytes(n * 8)
			for b.Loop() {
				CFIR(dstC, wc, hc, 1)
			}
		})
		b.Run("FIR/stride4/"+name, func(b *testing.B) {
			w := randF32(r, 4*n+taps)
			for b.Loop() {
				FIR(dst[:n], w, h, 4)
			}
		})
	}
}

func BenchmarkDot(b *testing.B) {
	r := rand.New(rand.NewSource(8))
	for _, n := range []int{16, 64, 256} {
		x := randF32(r, n)
		xc := randC64(r, n)
		c := randF32(r, n)
		b.Run("Dot/n="+strconv.Itoa(n), func(b *testing.B) {
			for b.Loop() {
				Dot(x, c)
			}
		})
		b.Run("CDotReal/n="+strconv.Itoa(n), func(b *testing.B) {
			for b.Loop() {
				CDotReal(xc, c)
			}
		})
	}
}
