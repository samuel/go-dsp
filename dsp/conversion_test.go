package dsp

import (
	"bytes"
	"testing"
)

const benchSize = 1 << 12

func TestUi8toi16(t *testing.T) {
	input := make([]byte, 300)
	for i := 0; i < len(input); i++ {
		input[i] = byte(i)
	}
	input = input[:256]
	output := make([]int16, len(input)+8)
	expected := make([]int16, len(input)+8)
	ui8toi16(input, expected) // Use Go implementation as reference
	Ui8toi16(input, output)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}

	// Unaligned input
	input = input[1:]
	output = make([]int16, len(input)+8)
	expected = make([]int16, len(input)+8)
	ui8toi16(input, expected) // Use Go implementation as reference
	Ui8toi16(input, output)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}
}

func TestUi8toi16b(t *testing.T) {
	input := make([]byte, 300)
	for i := 0; i < len(input); i++ {
		input[i] = byte(i)
	}
	input = input[:256]
	output := make([]byte, len(input)*2+16)
	expected := make([]byte, len(input)*2+16)
	ui8toi16b(input, expected) // Use Go implementation as reference
	Ui8toi16b(input, output)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
	}

	// Make sure unmatched input and output lengths don't cause a panic/segfault
	Ui8toi16b([]byte{}, []byte{1, 2})
	Ui8toi16b([]byte{1, 2}, []byte{1, 2})

	// Unaligned output (even), non 8-byte multiple input
	input = input[1:]
	output = make([]byte, len(input)*2+16)[2:]
	expected = make([]byte, len(input)*2+16)[2:]
	ui8toi16b(input, expected) // Use Go implementation as reference
	Ui8toi16b(input, output)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
	}

	// Unaligned output (odd), non 8-byte multiple input
	input = input[1:]
	output = make([]byte, len(input)*2+16)[1:]
	expected = make([]byte, len(input)*2+16)[1:]
	ui8toi16b(input, expected) // Use Go implementation as reference
	Ui8toi16b(input, output)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
	}
}

func TestUi8tof32(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		input := make([]byte, 300)
		for i := 0; i < len(input); i++ {
			input[i] = byte(i)
		}
		input = input[:256]
		output := make([]float32, len(input)+4)
		expected := make([]float32, len(input)+4)
		ui8tof32(input, expected) // Use Go implementation as reference
		Ui8tof32(input, output)
		for i := 0; i < len(output); i++ {
			if output[i] != expected[i] {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}

		// Unaligned
		input = input[1:]
		output = make([]float32, len(input)+4)[1:]
		expected = make([]float32, len(input)+4)[1:]
		ui8tof32(input, expected) // Use Go implementation as reference
		Ui8tof32(input, output)
		for i := 0; i < len(output); i++ {
			if output[i] != expected[i] {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}
	})
}

func TestI8tof32(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		input := make([]byte, 300)
		for i := 0; i < len(input); i++ {
			input[i] = byte(int8(i - 128))
		}
		input = input[:256]
		output := make([]float32, len(input)+4)
		expected := make([]float32, len(input)+4)
		i8tof32(input, expected) // Use Go implementation as reference
		I8tof32(input, output)
		for i := 0; i < len(output); i++ {
			if output[i] != expected[i] {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}

		// Unaligned
		input = input[1:]
		output = make([]float32, len(input)+4)[1:]
		expected = make([]float32, len(input)+4)[1:]
		i8tof32(input, expected) // Use Go implementation as reference
		I8tof32(input, output)
		for i := 0; i < len(output); i++ {
			if output[i] != expected[i] {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}
	})
}

func TestUi8toc64(t *testing.T) {
	input := []byte{0, 1, 192, 200, 212, 1, 2, 3}[:5]
	output := make([]complex64, len(input)/2+4)
	expected := make([]complex64, len(input)/2+4)
	ui8toc64(input, expected) // Use Go implementation as reference
	Ui8toc64(input, output)
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}

	// longer input
	input = []byte{0, 1, 192, 200, 1, 2, 3, 4, 5, 6, 7}
	output = make([]complex64, 2)
	expected = make([]complex64, 2)
	ui8toc64(input, expected) // Use Go implementation as reference
	Ui8toc64(input, output)
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}

	// longer output
	input = []byte{0, 1, 192, 200}
	output = make([]complex64, 4*10)
	expected = make([]complex64, 4*10)
	ui8toc64(input, expected) // Use Go implementation as reference
	Ui8toc64(input, output)
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}
}

func TestF32toi16(t *testing.T) {
	// Make sure there's non-zero value after the expected length of the slice
	// to detect out of bound access.
	input := make([]float32, 300)
	for i := 0; i < len(input); i++ {
		input[i] = 2.0*float32(i)/float32(len(input)) - 1.0
	}
	input = input[:256]
	output := make([]int16, len(input)+4)
	expected := make([]int16, len(input)+4)
	f32toi16(input, expected, 1<<13) // Use Go implementation as reference
	F32toi16(input, output, 1<<13)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}

	// Unaligned
	input = input[1:]
	output = make([]int16, len(input)+4)[1:]
	expected = make([]int16, len(input)+4)[1:]
	f32toi16(input, expected, 1<<13) // Use Go implementation as reference
	F32toi16(input, output, 1<<13)
	for i, v := range expected {
		if output[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
		}
	}
}

func TestF32toi16ble(t *testing.T) {
	// Make sure there's non-zero value after the expected length of the slice
	// to detect out of bound access.
	input := []float32{0.0, 1.0, -1.0, 2.13, -2.13, 2.0, 3.0, 4.0}[:5]
	output := make([]byte, len(input)*2+4)
	expected := make([]byte, len(input)*2+4)
	f32toi16ble(input, expected, 1<<13) // Use Go implementation as reference
	F32toi16ble(input, output, 1<<13)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
	}
}

func TestI16bleToF64(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		input := []byte{
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x80, 0x00, 0x00, 0xff,
		}
		output := make([]float64, len(input)/2)
		expected := make([]float64, len(output))
		i16bleToF64(input, expected, 1)
		I16bleToF64(input, output, 1)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}
		i16bleToF64(input, expected, 1.0/32768.0)
		I16bleToF64(input, output, 1.0/32768.0)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}

		// Unaligned
		input = input[1 : len(input)-1]
		output = make([]float64, len(input)/2+1)[1:]
		expected = make([]float64, len(output))
		i16bleToF64(input, expected, 1.0/32768.0)
		I16bleToF64(input, output, 1.0/32768.0)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}

		// Make sure there's non-zero value after the expected length of the slice
		// to detect out of bound access.
		input = []byte{0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80, 0x64, 0x00, 0xff, 0x00}[:10]
		output = make([]float64, len(input)/2+4)
		expected = make([]float64, len(output))
		i16bleToF64(input, expected, 1.0)
		I16bleToF64(input, output, 1.0)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}
	})
}

func TestI16bleToF32(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		input := []byte{
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x80, 0x00, 0x00, 0xff,
		}
		output := make([]float32, len(input)/2)
		expected := make([]float32, len(output))
		i16bleToF32(input, expected, 1)
		I16bleToF32(input, output, 1)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}
		i16bleToF32(input, expected, 1.0/32768.0)
		I16bleToF32(input, output, 1.0/32768.0)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}

		// Unaligned
		input = input[1 : len(input)-1]
		output = make([]float32, len(input)/2+1)[1:]
		expected = make([]float32, len(output))
		i16bleToF32(input, expected, 1.0/32768.0)
		I16bleToF32(input, output, 1.0/32768.0)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}

		// Make sure there's non-zero value after the expected length of the slice
		// to detect out of bound access.
		input = []byte{0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80, 0x64, 0x00, 0xff, 0x00}[:10]
		output = make([]float32, len(input)/2+4)
		expected = make([]float32, len(output))
		i16bleToF32(input, expected, 1.0)
		I16bleToF32(input, output, 1.0)
		for i, v := range expected {
			if output[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", output, expected)
			}
		}
	})
}

func BenchmarkUi8toi16(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]int16, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ui8toi16(input, output)
	}
}

func BenchmarkUi8toi16_Go(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]int16, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ui8toi16(input, output)
	}
}

func BenchmarkUi8toi16b(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]byte, len(input)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ui8toi16b(input, output)
	}
}

func BenchmarkUi8toi16b_Unaligned(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]byte, len(input)*2+3)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ui8toi16b(input, output)
	}
}

func BenchmarkUi8toi16b_Go(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]byte, len(input)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ui8toi16b(input, output)
	}
}

func BenchmarkUi8tof32(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ui8tof32(input, output)
	}
}

func BenchmarkUi8tof32_Unaligned(b *testing.B) {
	input := make([]byte, benchSize+1)[1:]
	output := make([]float32, len(input)+1)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ui8tof32(input, output)
	}
}

func BenchmarkUi8tof32_Go(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ui8tof32(input, output)
	}
}

func BenchmarkI8tof32(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		I8tof32(input, output)
	}
}

func BenchmarkI8tof32_Unaligned(b *testing.B) {
	input := make([]byte, benchSize+1)[1:]
	output := make([]float32, len(input)+1)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		I8tof32(input, output)
	}
}

func BenchmarkI8tof32_Go(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]float32, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		i8tof32(input, output)
	}
}

func BenchmarkUi8toc64(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]complex64, len(input)/2)
	b.SetBytes(benchSize / 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Ui8toc64(input, output)
	}
}

func BenchmarkUi8toc64_Go(b *testing.B) {
	input := make([]byte, benchSize)
	output := make([]complex64, len(input)/2)
	b.SetBytes(benchSize / 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ui8toc64(input, output)
	}
}

func BenchmarkF32toi16(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]int16, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		F32toi16(input, output, 1<<7)
	}
}

func BenchmarkF32toi16_Unaligned(b *testing.B) {
	input := make([]float32, benchSize+1)[1:]
	output := make([]int16, len(input)+1)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		F32toi16(input, output, 1<<7)
	}
}

func BenchmarkF32toi16_Go(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]int16, len(input))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f32toi16(input, output, 1<<7)
	}
}

func BenchmarkF32toi16ble(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]byte, len(input)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		F32toi16ble(input, output, 1<<7)
	}
}

func BenchmarkF32toi16ble_Go(b *testing.B) {
	input := make([]float32, benchSize)
	output := make([]byte, len(input)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f32toi16ble(input, output, 1<<7)
	}
}

func BenchmarkI16bleToF64(b *testing.B) {
	input := make([]byte, benchSize*2)
	output := make([]float64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		I16bleToF64(input, output, 1<<15)
	}
}

func BenchmarkI16bleToF64_Go(b *testing.B) {
	input := make([]byte, benchSize*2)
	output := make([]float64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		i16bleToF64(input, output, 1<<15)
	}
}

func BenchmarkI16bleToF32(b *testing.B) {
	input := make([]byte, benchSize*2)
	output := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		I16bleToF32(input, output, 1<<15)
	}
}

func BenchmarkI16bleToF32_Go(b *testing.B) {
	input := make([]byte, benchSize*2)
	output := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		i16bleToF32(input, output, 1<<15)
	}
}
