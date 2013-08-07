package sdr

import "testing"

func TestLowPassDownsampleComplexFilter(t *testing.T) {
	filter := &LowPassDownsampleComplexFilter{Downsample: 2}
	input := []complex64{complex(0.0, 0.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}
	output, _ := lowPassDownsampleComplexFilterAsm(filter, input)
	expected, _ := lowPassDownsampleComplexFilter(filter, input)
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
		_, _ = lowPassDownsampleComplexFilterAsm(filter, input)
	}
}

func BenchmarkLowPassDownsampleComplexFilter_Go(b *testing.B) {
	filter := &LowPassDownsampleComplexFilter{Downsample: 2}
	input := make([]complex64, 256)
	for i := 0; i < 256; i++ {
		input[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for i := 0; i < b.N; i++ {
		_, _ = lowPassDownsampleComplexFilter(filter, input)
	}
}
