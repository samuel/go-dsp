package sdr

import "testing"

func TestRotate90Filter(t *testing.T) {
	filter := &Rotate90Filter{}
	input := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}
	output, _ := rotate90FilterAsm(filter, input)
	expected, _ := rotate90Filter(filter, input)
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
	input := make([]complex64, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_, _ = rotate90FilterAsm(filter, input)
	}
}

func BenchmarkRotate90Filter_Go(b *testing.B) {
	filter := &Rotate90Filter{}
	input := make([]complex64, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_, _ = rotate90Filter(filter, input)
	}
}
