package f32

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestSumExactWhenNothingRounds pins the part of Sum's contract that survives
// the unspecified order: if every partial sum is exact, so is the result,
// whatever order the accumulators fold in.
func TestSumExactWhenNothingRounds(t *testing.T) {
	for _, n := range []int{0, 1, 7, 8, 9, 15, 16, 17, 63, 64, 65, 1000} {
		src := make([]float32, n)
		var want float32
		for i := range src {
			src[i] = float32(i%17) - 8 // small integers, exactly representable
			want += src[i]
		}
		if got := Sum(src); got != want {
			t.Errorf("n=%d: Sum = %v, want %v", n, got, want)
		}
	}
}

// TestSumBeatsSerial checks the accuracy claim in Sum's doc comment rather than
// taking it on trust. It is a statistical claim -- rounding errors are a random
// walk and a single input can go either way -- so it averages over many inputs.
func TestSumBeatsSerial(t *testing.T) {
	const (
		n      = 1 << 16
		trials = 40
	)
	var blockedTotal, serialTotal float64
	for seed := range uint64(trials) {
		r := rand.New(rand.NewPCG(seed, 99))
		src := make([]float32, n)
		var exact float64
		for i := range src {
			src[i] = float32(r.Float64() + 0.5)
			exact += float64(src[i])
		}
		var serial float32
		for _, v := range src {
			serial += v
		}
		blockedTotal += math.Abs(float64(Sum(src)) - exact)
		serialTotal += math.Abs(float64(serial) - exact)
	}
	blocked := blockedTotal / trials
	serial := serialTotal / trials
	t.Logf("mean |error| over %d inputs of %d: blocked %v, serial %v (%.1fx)",
		trials, n, blocked, serial, serial/blocked)
	if blocked >= serial {
		t.Errorf("blocked sum is not more accurate: %v against %v", blocked, serial)
	}
}

func TestMaxIdx(t *testing.T) {
	cases := []struct {
		name string
		src  []float32
		val  float32
		idx  int
	}{
		{"empty", nil, float32(math.Inf(-1)), -1},
		{"single", []float32{3}, 3, 0},
		{"first", []float32{9, 1, 2}, 9, 0},
		{"last", []float32{1, 2, 9}, 9, 2},
		{"tie takes the lowest index", []float32{1, 7, 7, 3}, 7, 1},
		{"all negative", []float32{-5, -2, -9}, -2, 1},
	}
	for _, c := range cases {
		v, i := MaxIdx(c.src)
		if v != c.val || i != c.idx {
			t.Errorf("%s: MaxIdx = (%v, %d), want (%v, %d)", c.name, v, i, c.val, c.idx)
		}
	}
	// The value must be the one at the index it reports, and agree with Max.
	r := rand.New(rand.NewPCG(3, 4))
	for _, n := range []int{1, 5, 33, 257} {
		src := make([]float32, n)
		for i := range src {
			src[i] = float32(r.Float64()*20 - 10)
		}
		v, i := MaxIdx(src)
		if src[i] != v {
			t.Errorf("n=%d: reported %v at %d, slice holds %v", n, v, i, src[i])
		}
		if m := Max(src); m != v {
			t.Errorf("n=%d: MaxIdx gave %v, Max gave %v", n, v, m)
		}
	}
}

// TestCMagSqMatchesCAbs pins the two against each other: CMagSq is exactly what
// CAbs takes the square root of, so their explicit roundings have to stay in
// step. A tolerance would hide the FMA contraction the roundings exist to block.
func TestCMagSqMatchesCAbs(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, n := range []int{0, 1, 3, 16, 17, 64, 129} {
		src := make([]complex64, n)
		for i := range src {
			src[i] = complex(float32(r.Float64()*8-4), float32(r.Float64()*8-4))
		}
		sq := make([]float32, n)
		mag := make([]float32, n)
		CMagSq(sq, src)
		CAbs(mag, src)
		for i, v := range src {
			re, im := real(v), imag(v)
			want := float32(re*re) + float32(im*im)
			if sq[i] != want {
				t.Fatalf("n=%d [%d]: CMagSq = %v, want %v", n, i, sq[i], want)
			}
			if got := float32(math.Sqrt(float64(sq[i]))); mag[i] != got {
				t.Fatalf("n=%d [%d]: CAbs = %v, sqrt(CMagSq) = %v", n, i, mag[i], got)
			}
		}
	}
	// The shortest slice bounds the work.
	dst := make([]float32, 4)
	CMagSq(dst, []complex64{complex(3, 4)})
	if dst[0] != 25 || dst[1] != 0 {
		t.Errorf("short src: %v", dst)
	}
}
