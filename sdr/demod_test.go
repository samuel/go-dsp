package sdr

import "testing"

func TestFMDemodulation(t *testing.T) {
	filter := &FMDemodFilter{}
	input := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}
	output := make([]float32, len(input))
	filter.pre = 0.0
	fmDemodulateAsm(filter, input, output)
	expected := make([]float32, len(input))
	filter.pre = 0.0
	fmDemodulate(filter, input, expected)
	if len(output) != len(expected) {
		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
	}
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
		}
	}
}

func BenchmarkTestFMDemodulation(b *testing.B) {
	filter := &FMDemodFilter{}
	input := make([]complex64, 256)
	output := make([]float32, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_, _ = fmDemodulateAsm(filter, input, output)
	}
}

func BenchmarkTestFMDemodulation_Go(b *testing.B) {
	filter := &FMDemodFilter{}
	input := make([]complex64, 256)
	output := make([]float32, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_, _ = fmDemodulate(filter, input, output)
	}
}
