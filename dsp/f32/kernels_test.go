package f32

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestScale(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		const scale = 1.0 / 256.0
		// Scale is vectorized and unrolls by a non-trivial amount, so check
		// every length across a couple of unrolled blocks against a scalar
		// reference to catch mishandled tails.
		for n := range 200 {
			src := make([]float32, n)
			expected := make([]float32, n)
			for i := range src {
				src[i] = float32(i)
				expected[i] = float32(i) * scale
			}

			dst := make([]float32, n)
			Scale(dst, src, scale)
			for i, v := range expected {
				if dst[i] != v {
					t.Fatalf("len %d: dst doesn't match expected:\n%+v\n%+v", n, dst, expected)
				}
			}

			// Unaligned input and output
			unaligned := make([]float32, n+1)[1:]
			copy(unaligned, src)
			dst = make([]float32, n+1)[1:]
			Scale(dst, unaligned, scale)
			for i, v := range expected {
				if dst[i] != v {
					t.Fatalf("len %d unaligned: dst doesn't match expected:\n%+v\n%+v", n, dst, expected)
				}
			}
		}

		// Mismatched lengths: only min(len(src), len(dst)) elements are
		// read and written.
		src := make([]float32, 64)
		for i := range src {
			src[i] = float32(i + 1)
		}
		for _, outLen := range []int{40, 64, 96} {
			dst := make([]float32, outLen)
			Scale(dst, src, scale)
			for i, v := range dst {
				var want float32
				if i < len(src) {
					want = src[i] * scale
				}
				if v != want {
					t.Fatalf("src %d dst %d: dst[%d] = %v, want %v", len(src), outLen, i, v, want)
				}
			}
		}
	})
}

// absC64Ref is the exact reference every CAbs test compares against.
//
// The float32 conversions are required: without them Go may contract the
// expression into one FMA (arm64 does), making this disagree with the vector
// implementations for about 8% of random inputs. See vabsC64Scalar.
func absC64Ref(v complex64) float32 {
	re, im := real(v), imag(v)
	return float32(math.Sqrt(float64(float32(re*re) + float32(im*im))))
}

func TestCAbs(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		// Every length across a couple of unrolled blocks, checked exactly
		// against the scalar reference, so that a mishandled tail cannot hide.
		// Past three full blocks of the widest implementation, which is 64
		// magnitudes on the amd64 512-bit path.
		r := rand.New(rand.NewPCG(3, 4))
		for n := range 200 {
			src := make([]complex64, n)
			expected := make([]float32, n)
			for i := range src {
				src[i] = complex(r.Float32()*4-2, r.Float32()*4-2)
				expected[i] = absC64Ref(src[i])
			}

			dst := make([]float32, n)
			CAbs(dst, src)
			for i, v := range expected {
				if dst[i] != v {
					t.Fatalf("len %d: dst[%d] = %v, want %v (src %v)", n, i, dst[i], v, src[i])
				}
			}

			// Unaligned input and output
			ui := make([]complex64, n+1)[1:]
			copy(ui, src)
			uo := make([]float32, n+1)[1:]
			CAbs(uo, ui)
			for i, v := range expected {
				if uo[i] != v {
					t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, uo[i], v)
				}
			}
		}

		// Empty and nil slices, which the reference this replaced indexed out
		// of range.
		CAbs(nil, nil)
		CAbs([]float32{}, []complex64{})

		// Mismatched lengths: only min(len(src), len(dst)) is touched.
		src := make([]complex64, 64)
		for i := range src {
			src[i] = complex(float32(i+1), float32(i+2))
		}
		for _, outLen := range []int{40, 64, 96} {
			dst := make([]float32, outLen)
			CAbs(dst, src)
			for i, v := range dst {
				var want float32
				if i < len(src) {
					want = absC64Ref(src[i])
				}
				if v != want {
					t.Fatalf("src 64 dst %d: dst[%d] = %v, want %v", outLen, i, v, want)
				}
			}
		}

		// Values worth naming: zero, a purely real and a purely imaginary
		// value, so that a swapped real/imaginary lane in the deinterleave
		// shows up, and one exact answer.
		named := []complex64{
			complex(0.0, 0.0),
			complex(1.0, 0.0),
			complex(0.0, -1.0),
			complex(1.3, -2.7),
			complex(-2.3, 1.9),
			complex(3.0, 4.0),
		}
		out := make([]float32, len(named))
		CAbs(out, named)
		for i, v := range named {
			if want := absC64Ref(v); out[i] != want {
				t.Fatalf("%v: got %v want %v", v, out[i], want)
			}
		}
		if out[5] != 5.0 {
			t.Fatalf("|3+4i| = %v, want 5", out[5])
		}
	})
}

func TestMax(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		// Max is vectorized over several accumulators and unrolls by a
		// non-trivial amount, so check every length against a scalar reference
		// to catch mishandled tails. Past three full blocks of the largest
		// implementation, which is 128 floats on amd64, so that the main loop,
		// the intermediate step and every remainder length are all covered.
		r := rand.New(rand.NewPCG(1, 2))
		for n := range 400 {
			src := make([]float32, n)
			expected := float32(math.Inf(-1))
			for i := range src {
				src[i] = r.Float32() - 0.5
				expected = max(expected, src[i])
			}
			if mx := Max(src); mx != expected {
				t.Fatalf("len %d: expected %v got %v", n, expected, mx)
			}

			// Unaligned input
			unaligned := make([]float32, n+1)[1:]
			copy(unaligned, src)
			if mx := Max(unaligned); mx != expected {
				t.Fatalf("len %d unaligned: expected %v got %v", n, expected, mx)
			}

			// The remainder is folded in by re-reading the last full vector, so
			// put the maximum where only that read can find it.
			if n > 0 {
				tail := make([]float32, n)
				tail[n-1] = 1.0
				if mx := Max(tail); mx != 1.0 {
					t.Fatalf("len %d last element: expected 1 got %v", n, mx)
				}
			}
		}

		// The maximum in each individual lane, so that a dropped accumulator or
		// a botched final reduction cannot hide.
		for i := range 1024 {
			src := make([]float32, 1024)
			src[i] = 1.0
			if mx := Max(src); mx != 1.0 {
				t.Fatalf("expected 1.0 got %f at position %d", mx, i)
			}
		}

		// Empty slice
		if mx := Max(nil); mx != float32(math.Inf(-1)) {
			t.Fatalf("expected -Inf got %f", mx)
		}

		// All less than zero, so that the -Inf identity value is the only thing
		// keeping a zeroed accumulator from winning.
		src := []float32{-17, -16, -15, -14, -13, -12, -11, -10, -9, -8, -7, -6, -5, -4, -3, -2, -1}
		if mx := Max(src); mx != -1.0 {
			t.Fatalf("expected -1.0 got %f", mx)
		}

		// Ascending
		src = []float32{-4.0, -3.0, -2.0, -1.0, 0.0, 1.0, 2.0, 3.0, 4.0}
		if mx := Max(src); mx != 4.0 {
			t.Fatalf("expected 4.0 got %f", mx)
		}

		// Descending
		src = []float32{4.0, 3.0, 2.0, 1.0, 0.0, -1.0, -2.0, -3.0, -4.0}
		if mx := Max(src); mx != 4.0 {
			t.Fatalf("expected 4.0 got %f", mx)
		}

		// Unordered
		src = []float32{1.5, -4.0, 8.0, 0.0, -1.0, 2.0, -3.0}
		if mx := Max(src); mx != 8.0 {
			t.Fatalf("expected 8.0 got %f", mx)
		}
	})
}

func TestMin(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		// The mirror of TestMax above; see the note there for why every
		// length up to three blocks of the largest implementation is checked.
		r := rand.New(rand.NewPCG(1, 2))
		for n := range 400 {
			src := make([]float32, n)
			expected := float32(math.Inf(1))
			for i := range src {
				src[i] = r.Float32() - 0.5
				expected = min(expected, src[i])
			}
			if mn := Min(src); mn != expected {
				t.Fatalf("len %d: expected %v got %v", n, expected, mn)
			}

			// Unaligned input
			unaligned := make([]float32, n+1)[1:]
			copy(unaligned, src)
			if mn := Min(unaligned); mn != expected {
				t.Fatalf("len %d unaligned: expected %v got %v", n, expected, mn)
			}

			// The remainder is folded in by re-reading the last full vector, so
			// put the minimum where only that read can find it.
			if n > 0 {
				tail := make([]float32, n)
				tail[n-1] = -1.0
				if mn := Min(tail); mn != -1.0 {
					t.Fatalf("len %d last element: expected -1 got %v", n, mn)
				}
			}
		}

		// The minimum in each individual lane, so that a dropped accumulator or
		// a botched final reduction cannot hide.
		for i := range 1024 {
			src := make([]float32, 1024)
			src[i] = -1.0
			if mn := Min(src); mn != -1.0 {
				t.Fatalf("expected -1.0 got %f at position %d", mn, i)
			}
		}

		// Empty slice
		if mn := Min(nil); mn != float32(math.Inf(1)) {
			t.Fatalf("expected +Inf got %f", mn)
		}

		// All greater than zero, so that the +Inf identity value is the only
		// thing keeping a zeroed accumulator from winning.
		src := []float32{17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
		if mn := Min(src); mn != 1.0 {
			t.Fatalf("expected 1.0 got %f", mn)
		}

		// Ascending
		src = []float32{-4.0, -3.0, -2.0, -1.0, 0.0, 1.0, 2.0, 3.0, 4.0}
		if mn := Min(src); mn != -4.0 {
			t.Fatalf("expected -4.0 got %f", mn)
		}

		// Descending
		src = []float32{4.0, 3.0, 2.0, 1.0, 0.0, -1.0, -2.0, -3.0, -4.0}
		if mn := Min(src); mn != -4.0 {
			t.Fatalf("expected -4.0 got %f", mn)
		}

		// Unordered
		src = []float32{1.5, -4.0, 8.0, 0.0, -1.0, 2.0, -3.0}
		if mn := Min(src); mn != -4.0 {
			t.Fatalf("expected -4.0 got %f", mn)
		}
	})
}

// simdBenchSizes spans working sets from L1-resident to memory-bound: loop
// overhead changes are only visible while buffers fit in cache. From ~16K
// floats up the loops are bandwidth-bound and every implementation converges,
// so a single large size cannot see that class of change.
//
// 1K-1 is deliberately not a multiple of any block size, so it exercises the
// vector tail and scalar remainder the power-of-two sizes skip.
var simdBenchSizes = []struct {
	name string
	n    int
}{
	{"64", 64},
	{"256", 256},
	{"1K", 1024},
	{"1K-1", 1023},
	{"4K", 4096},
	{"16K", 16384},
	{"64K", 65536},
}

func BenchmarkScale(b *testing.B) {
	for _, sz := range simdBenchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]float32, sz.n)
			dst := make([]float32, sz.n)
			// SetBytes reports real bytes, not elements, so the MB/s figure can
			// be read against the machine's cache and memory bandwidth — where
			// the loop stops being compute-bound.
			b.SetBytes(int64(sz.n) * 4)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				Scale(dst, src, 1.0/float32(sz.n))
			}
		})
	}
}

// SetBytes reports the input bytes, eight per complex64; the output adds four
// more, so the traffic is 1.5x the stated MB/s.
func BenchmarkCAbs(b *testing.B) {
	for _, sz := range simdBenchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]complex64, sz.n)
			dst := make([]float32, sz.n)
			r := rand.New(rand.NewPCG(0, 0))
			for i := range src {
				src[i] = complex(r.Float32()*4-2, r.Float32()*4-2)
			}
			b.SetBytes(int64(sz.n) * 8)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				CAbs(dst, src)
			}
		})
	}
}

func BenchmarkMax(b *testing.B) {
	for _, sz := range simdBenchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]float32, sz.n)
			r := rand.New(rand.NewPCG(0, 0))
			for i := range src {
				src[i] = r.Float32()
			}
			b.SetBytes(int64(sz.n) * 4)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = Max(src)
			}
		})
	}
}

func BenchmarkMin(b *testing.B) {
	for _, sz := range simdBenchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]float32, sz.n)
			r := rand.New(rand.NewPCG(0, 0))
			for i := range src {
				src[i] = r.Float32()
			}
			b.SetBytes(int64(sz.n) * 4)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = Min(src)
			}
		})
	}
}
