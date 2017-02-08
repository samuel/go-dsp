package dsp

import (
	"math"
	"math/rand"
	"testing"
)

const (
	approxErrorLimit = 0.011
)

var (
	atanBenchTable      = [][2]float32{}
	atanBenchTableFixed = [][2]int{}
)

func init() {
	for y := -1.0; y <= 1.0; y += 0.5 {
		for x := -1.0; x <= 1.0; x += 0.5 {
			atanBenchTable = append(atanBenchTable, [2]float32{float32(x), float32(y)})
			atanBenchTableFixed = append(atanBenchTableFixed, [2]int{int(x * (1 << 14)), int(y * (1 << 14))})
		}
	}
}

func TestAtan2(t *testing.T) {
	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			expected := float32(math.Atan2(y, x))
			if err := math.Abs(float64(expected - FastAtan2(float32(y), float32(x)))); err > approxErrorLimit {
				t.Errorf("FastAtan2 gave an error of %f for x=%f y=%f", err, x, y)
			}
			if err := math.Abs(float64(expected - FastAtan2_2(float32(y), float32(x)))); err > approxErrorLimit {
				t.Errorf("FastAtan2_2 gave an error of %f for x=%f y=%f", err, x, y)
			}
		}
	}
	x, y := 0.0, 0.0
	expected := float32(math.Atan2(y, x))
	if err := math.Abs(float64(expected - FastAtan2(float32(y), float32(x)))); err > approxErrorLimit {
		t.Errorf("FastAtan2 gave an error of %f for x=%f y=%f", err, x, y)
	}
	if err := math.Abs(float64(expected - FastAtan2_2(float32(y), float32(x)))); err > approxErrorLimit {
		t.Errorf("FastAtan2_2 gave an error of %f for x=%f y=%f", err, x, y)
	}
}

func TestFastAtan2Error(t *testing.T) {
	maxE := 0.0
	sumE := 0.0
	count := 0
	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			ai := float64(FastAtan2(float32(y), float32(x)))
			af := math.Atan2(y, x)
			e := math.Abs(ai - af)
			sumE += e
			if e > maxE {
				maxE = e
			}
			count++
		}
	}
	if maxE > 0.0102 {
		t.Errorf("Expected max error of 0.0102 got %f", maxE)
	}
	t.Logf("Max error %f\n", maxE)
	t.Logf("Mean absolute error %f", sumE/float64(count))
}

func TestFastAtan2_2Error(t *testing.T) {
	maxE := 0.0
	sumE := 0.0
	count := 0
	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			ai := float64(FastAtan2_2(float32(y), float32(x)))
			af := math.Atan2(y, x)
			e := math.Abs(ai - af)
			sumE += e
			if e > maxE {
				maxE = e
			}
			count++
		}
	}
	if maxE > 0.005 {
		t.Errorf("Expected max error of 0.005 got %f", maxE)
	}
	t.Logf("Max error %f\n", maxE)
	t.Logf("Mean absolute error %f", sumE/float64(count))
}

func TestVScaleF32(t *testing.T) {
	input := make([]float32, 257)
	for i := 0; i < len(input); i++ {
		input[i] = float32(i)
	}
	expected := make([]float32, len(input))
	output := make([]float32, len(input))
	vscaleF32(input, expected, 1.0/256.0)
	VScaleF32(input, output, 1.0/256.0)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}

	// Unaligned
	input = input[1:]
	expected = make([]float32, len(input)+1)[1:]
	output = make([]float32, len(input)+1)[1:]
	vscaleF32(input, expected, 1.0/256.0)
	VScaleF32(input, output, 1.0/256.0)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}
}

func TestVAbsC64(t *testing.T) {
	input := []complex64{
		complex(0.0, 0.0),
		complex(1.0, 1.0),
		complex(1.3, -2.7),
		complex(0.0, -1.0),
		complex(1.0, 0.0),
		complex(-2.3, 1.9),
	}
	expected := make([]float32, len(input))
	for i, v := range input {
		expected[i] = float32(math.Sqrt(float64(real(v)*real(v) + imag(v)*imag(v))))
	}
	output := make([]float32, len(input))
	VAbsC64(input, output)
	for i, v := range output {
		if !approxEqual32(v, expected[i], 1e-20) {
			t.Errorf("Expected %+v got %+v for %+v", expected[i], v, input[i])
		}
	}
}

func TestVMaxF32(t *testing.T) {
	input := make([]float32, 123)
	for i := 0; i < len(input); i++ {
		input[i] = rand.Float32() - 0.5
	}
	expected := vMaxF32(input)
	max := VMaxF32(input)
	if max != expected {
		t.Fatalf("Expected %f got %f", expected, max)
	}

	// Ascending
	input = []float32{-4.0, -3.0, -2.0, -1.0, 0.0, 1.0, 2.0, 3.0, 4.0}
	if max := VMaxF32(input); max != 4.0 {
		t.Fatalf("Expected 4.0 got %f", max)
	}

	// Descending
	input = []float32{4.0, 3.0, 2.0, 1.0, 0.0, -1.0, -2.0, -3.0, -4.0}
	if max := VMaxF32(input); max != 4.0 {
		t.Fatalf("Expected 4.0 got %f", max)
	}

	// Unordered
	input = []float32{1.5, -4.0, 8.0, 0.0, -1.0, 2.0, -3.0}
	if max := VMaxF32(input); max != 8.0 {
		t.Fatalf("Expected 8.0 got %f", max)
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
		for _, xy := range atanBenchTable {
			FastAtan2(xy[1], xy[0])
		}
	}
}

func BenchmarkFastAtan2_Go(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, xy := range atanBenchTable {
			fastAtan2(xy[1], xy[0])
		}
	}
}

func BenchmarkFastAtan2_2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, xy := range atanBenchTable {
			FastAtan2_2(xy[1], xy[0])
		}
	}
}

func BenchmarkFastAtan2_2_Go(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, xy := range atanBenchTable {
			fastAtan2_2(xy[1], xy[0])
		}
	}
}

func BenchmarkAtan2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for _, xy := range atanBenchTable {
			math.Atan2(float64(xy[1]), float64(xy[0]))
		}
	}
}

func BenchmarkVScaleF32(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VScaleF32(input, output, 1.0/benchSize)
	}
}

func BenchmarkVScaleF32_Go(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vscaleF32(input, output, 1.0/benchSize)
	}
}

func BenchmarkVAbsC64(b *testing.B) {
	input := make([]complex64, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VAbsC64(input, output)
	}
}

func BenchmarkVAbsC64_Go(b *testing.B) {
	input := make([]complex64, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vAbsC64(input, output)
	}
}

func BenchmarkVMaxF32_Random(b *testing.B) {
	input := make([]float32, benchSize)
	rand.Seed(0)
	for i := 0; i < len(input); i++ {
		input[i] = rand.Float32()
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VMaxF32(input)
	}
}

func BenchmarkVMaxF32_Ascending(b *testing.B) {
	input := make([]float32, benchSize)
	for i := 0; i < len(input); i++ {
		input[i] = float32(i)
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VMaxF32(input)
	}
}

func BenchmarkVMaxF32_Descending(b *testing.B) {
	input := make([]float32, benchSize)
	for i := 0; i < len(input); i++ {
		input[i] = float32(-i)
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VMaxF32(input)
	}
}

func BenchmarkVMaxF32_Alternating(b *testing.B) {
	input := make([]float32, benchSize)
	for i := 0; i < len(input); i++ {
		if i&1 == 0 {
			input[i] = float32(i)
		} else {
			input[i] = float32(-i)
		}
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = VMaxF32(input)
	}
}

func BenchmarkVMaxF32_Go_Random(b *testing.B) {
	input := make([]float32, benchSize)
	rand.Seed(0)
	for i := 0; i < len(input); i++ {
		input[i] = rand.Float32()
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vMaxF32(input)
	}
}

func BenchmarkVMaxF32_Go_Ascending(b *testing.B) {
	input := make([]float32, benchSize)
	for i := 0; i < len(input); i++ {
		input[i] = float32(i)
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vMaxF32(input)
	}
}

func BenchmarkVMaxF32_Go_Decending(b *testing.B) {
	input := make([]float32, benchSize)
	for i := 0; i < len(input); i++ {
		input[i] = float32(-i)
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vMaxF32(input)
	}
}

func BenchmarkVMaxF32_Go_Alternating(b *testing.B) {
	input := make([]float32, benchSize)
	for i := 0; i < len(input); i++ {
		if i&1 == 0 {
			input[i] = float32(i)
		} else {
			input[i] = float32(-i)
		}
	}
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vMaxF32(input)
	}
}
