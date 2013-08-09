package sdr

import (
	// "math/rand"
	"testing"
)

// func TestFMDemodulation(t *testing.T) {
// 	filter := &FMDemodFilter{}
// 	input := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}
// 	output := make([]float32, len(input))
// 	filter.pre = 0.0
// 	fmDemodulateAsm(filter, input, output)
// 	expected := make([]float32, len(input))
// 	filter.pre = 0.0
// 	fmDemodulate(filter, input, expected)
// 	if len(output) != len(expected) {
// 		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
// 	}
// 	for i := 0; i < len(output); i++ {
// 		if output[i] != expected[i] {
// 			t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
// 		}
// 	}
// }

// func TestPolarDiscriminator32(t *testing.T) {
// 	for i := 0; i < 1000; i++ {
// 		x := complex(rand.Float32()-0.5, rand.Float32()-0.5)
// 		y := complex(rand.Float32()-0.5, rand.Float32()-0.5)
// 		expected := polarDiscriminator32(x, y)
// 		output := PolarDiscriminator32(x, y)
// 		if expected != output {
// 			t.Fatalf("Output differs: %f != %f", output, expected)
// 		}
// 	}
// }

func BenchmarkPolarDiscriminator32(b *testing.B) {
	x := complex(float32(1), float32(2))
	y := complex(float32(-3), float32(9))
	for i := 0; i < b.N; i++ {
		_ = PolarDiscriminator32(x, y)
	}
}

func BenchmarkFMDemodulation(b *testing.B) {
	filter := &FMDemodFilter{}
	input := make([]complex64, 256)
	output := make([]float32, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_, _ = filter.Demodulate(input, output)
	}
}

// func BenchmarkFMDemodulation_Go(b *testing.B) {
// 	filter := &FMDemodFilter{}
// 	input := make([]complex64, 256)
// 	output := make([]float32, 256)
// 	for i := 0; i < 256; i++ {
// 		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
// 	}
// 	for i := 0; i < b.N; i++ {
// 		_, _ = fmDemodulate(filter, input, output)
// 	}
// }
