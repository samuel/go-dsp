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

func TestScaleF32(t *testing.T) {
	input := make([]float32, 257)
	for i := 0; i < len(input); i++ {
		input[i] = float32(i)
	}
	expected := make([]float32, len(input))
	output := make([]float32, len(input))
	scalef32(input, expected, 1.0/256.0)
	Scalef32(input, output, 1.0/256.0)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}

	// Unaligned
	input = input[1:]
	expected = make([]float32, len(input)+1)[1:]
	output = make([]float32, len(input)+1)[1:]
	scalef32(input, expected, 1.0/256.0)
	Scalef32(input, output, 1.0/256.0)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
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

func BenchmarkScalef32(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Scalef32(input, output, 1.0/benchSize)
	}
}

func BenchmarkScalef32_Go(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scalef32(input, output, 1.0/benchSize)
	}
}
