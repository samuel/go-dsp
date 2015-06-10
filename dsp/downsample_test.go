package dsp

import "testing"

func TestLowPassDownsampleComplexFilter(t *testing.T) {
	filter := &LowPassDownsampleComplexFilter{Downsample: 2}
	input := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}

	output := make([]complex64, 256)
	copy(output, input)
	filter.now = 0.0
	filter.prevIndex = 0
	output = lowPassDownsampleComplexFilterAsm(filter, output)

	expected := make([]complex64, 256)
	copy(expected, input)
	filter.now = 0.0
	filter.prevIndex = 0
	expected = lowPassDownsampleComplexFilter(filter, expected)

	if len(output) != len(expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
	}
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}
}

func TestLowPassDownsampleRationalFilter(t *testing.T) {
	filter := &LowPassDownsampleRationalFilter{Fast: 3, Slow: 2}
	input := make([]float32, 256)
	for i := 0; i < len(input); i++ {
		input[i] = float32(i - 128)
	}

	output := make([]float32, 256)
	copy(output, input)
	filter.prevIndex = 0
	filter.sum = 0.0
	output = lowPassDownsampleRationalFilterAsm(filter, output)

	expected := make([]float32, 256)
	copy(expected, input)
	filter.prevIndex = 0
	filter.sum = 0.0
	expected = lowPassDownsampleRationalFilter(filter, expected)

	if len(output) != len(expected) {
		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
	}
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
		}
	}
}

func BenchmarkLowPassDownsampleComplexFilter(b *testing.B) {
	filter := &LowPassDownsampleComplexFilter{Downsample: 2}
	input := make([]complex64, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_ = lowPassDownsampleComplexFilterAsm(filter, input)
	}
}

func BenchmarkLowPassDownsampleComplexFilter_Go(b *testing.B) {
	filter := &LowPassDownsampleComplexFilter{Downsample: 2}
	input := make([]complex64, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_ = lowPassDownsampleComplexFilter(filter, input)
	}
}

func BenchmarkLowPassDownsampleRationalFilter(b *testing.B) {
	filter := &LowPassDownsampleRationalFilter{Fast: 3, Slow: 2}
	input := make([]float32, 256)
	for i := 0; i < 256; i++ {
		input[i] = float32(i) - 128.0
	}
	for i := 0; i < b.N; i++ {
		_ = lowPassDownsampleRationalFilterAsm(filter, input)
	}
}

func BenchmarkLowPassDownsampleRationalFilter_Go(b *testing.B) {
	filter := &LowPassDownsampleRationalFilter{Fast: 3, Slow: 2}
	input := make([]float32, 256)
	for i := 0; i < 256; i++ {
		input[i] = float32(i) - 128.0
	}
	for i := 0; i < b.N; i++ {
		_ = lowPassDownsampleRationalFilter(filter, input)
	}
}
