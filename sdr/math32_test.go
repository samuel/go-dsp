package sdr

import (
	"math"
	"math/rand"
	"testing"
)

const approxErrorLimit = 0.011

func TestAtan2(t *testing.T) {
	for i := 0; i < 1000; i++ {
		x := rand.Float32() - 0.5
		y := rand.Float32() - 0.5
		expected := float32(math.Atan2(float64(y), float64(x)))
		if err := math.Abs(float64(expected - FastAtan2(y, x))); err > approxErrorLimit {
			t.Errorf("FastArctan2 gave an error of %f for x=%f y=%f", err, x, y)
			// } else {
			// 	t.Logf("FastArctan2 error %f", err)
		}
		if err := math.Abs(float64(expected - FastAtan2_2(y, x))); err > approxErrorLimit {
			t.Errorf("FastArctan2_2 gave an error of %f for x=%f y=%f", err, x, y)
			// } else {
			// 	t.Logf("FastArctan2 error %f", err)
		}
	}
}

func BenchmarkConj32(b *testing.B) {
	in := complex64(complex(1.0, -0.2))
	for i := 0; i < b.N; i++ {
		_ = Conj32(in)
	}
}

func BenchmarkFastAtan2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		switch i & 3 {
		case 0:
			FastAtan2(1.0, 1.0)
		case 1:
			FastAtan2(-1.0, 1.0)
		case 2:
			FastAtan2(-1.0, -1.0)
		case 3:
			FastAtan2(1.0, -1.0)
		}
	}
}

func BenchmarkFastAtan2_Go(b *testing.B) {
	for i := 0; i < b.N; i++ {
		switch i & 3 {
		case 0:
			fastAtan2(1.0, 1.0)
		case 1:
			fastAtan2(-1.0, 1.0)
		case 2:
			fastAtan2(-1.0, -1.0)
		case 3:
			fastAtan2(1.0, -1.0)
		}
	}
}

func BenchmarkFastAtan2_2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		switch i & 3 {
		case 0:
			FastAtan2_2(1.0, 1.0)
		case 1:
			FastAtan2_2(-1.0, 1.0)
		case 2:
			FastAtan2_2(-1.0, -1.0)
		case 3:
			FastAtan2_2(1.0, -1.0)
		}
	}
}

func BenchmarkAtan2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		switch i & 3 {
		case 0:
			math.Atan2(1.0, 1.0)
		case 1:
			math.Atan2(-1.0, 1.0)
		case 2:
			math.Atan2(-1.0, -1.0)
		case 3:
			math.Atan2(1.0, -1.0)
		}
	}
}
