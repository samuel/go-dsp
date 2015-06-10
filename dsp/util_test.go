package dsp

import (
	"math"
	"testing"
)

func approxEqual(a, b, e float64) bool {
	return math.Abs(a-b) <= e
}

func approxEqual32(a, b, e float32) bool {
	return math.Abs(float64(a)-float64(b)) <= float64(e)
}

func TestRotate90Filter(t *testing.T) {
	filter := &Rotate90Filter{}
	input := make([]complex64, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	output := make([]complex64, 256)
	copy(output, input)
	output = rotate90FilterAsm(filter, output)
	expected := make([]complex64, 256)
	copy(expected, input)
	expected = rotate90Filter(filter, expected)
	if len(output) != len(expected) {
		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
	}
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
		}
	}
}

func BenchmarkRotate90Filter(b *testing.B) {
	filter := &Rotate90Filter{}
	input := make([]complex64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rotate90FilterAsm(filter, input)
	}
}

func BenchmarkRotate90Filter_Go(b *testing.B) {
	filter := &Rotate90Filter{}
	input := make([]complex64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rotate90Filter(filter, input)
	}
}

func BenchmarkI32Rotate90Filter(b *testing.B) {
	filter := &I32Rotate90Filter{}
	input := make([]int32, 2*benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = i32Rotate90FilterAsm(filter, input)
	}
}

func BenchmarkI32Rotate90Filter_Go(b *testing.B) {
	filter := &I32Rotate90Filter{}
	input := make([]int32, 2*benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = i32Rotate90Filter(filter, input)
	}
}
