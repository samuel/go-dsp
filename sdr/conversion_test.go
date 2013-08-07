package sdr

import (
	"bytes"
	"testing"
)

func TestUi8toi16b(t *testing.T) {
	input := make([]byte, 256)
	for i := 0; i < 256; i++ {
		input[i] = byte(i)
	}
	output := make([]byte, len(input)*2+4)
	expected := make([]byte, len(input)*2+4)
	ui8toi16b(input, expected)
	Ui8toi16b(input, output)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
	}

	// Make sure unmatched input and output lengths don't cause a panic/segfault
	Ui8toi16b([]byte{}, []byte{1, 2})
	Ui8toi16b([]byte{1, 2}, []byte{1, 2})
}

func TestF32toi16b(t *testing.T) {
	input := []float32{0.0, 1.0, -1.0, 10.0, -10.0}
	output := make([]byte, len(input)*2+4)
	expected := make([]byte, len(input)*2+4)
	f32toi16b(input, expected, 1<<13)
	F32toi16b(input, output, 1<<13)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
	}
}

func TestUi8toc64(t *testing.T) {
	input := []byte{0, 1, 192, 200}
	output := make([]complex64, len(input)/2+4)
	expected := make([]complex64, len(input)/2+4)
	ui8toc64(input, expected)
	Ui8toc64(input, output)
	for i := 0; i < len(output); i++ {
		if output[i] != expected[i] {
			t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
		}
	}
}

func BenchmarkUi8toi16b(b *testing.B) {
	input := make([]byte, 256)
	for i := 0; i < 256; i++ {
		input[i] = byte(i)
	}
	output := make([]byte, len(input)*2)
	for i := 0; i < b.N; i++ {
		Ui8toi16b(input, output)
	}
}

func BenchmarkUi8toi16b_Go(b *testing.B) {
	input := make([]byte, 256)
	for i := 0; i < 256; i++ {
		input[i] = byte(i)
	}
	output := make([]byte, len(input)*2)
	for i := 0; i < b.N; i++ {
		ui8toi16b(input, output)
	}
}
