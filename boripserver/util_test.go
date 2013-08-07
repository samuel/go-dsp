package main

import (
	"bytes"
	"testing"
)

func TestUi8toi16(t *testing.T) {
	input := make([]byte, 256)
	for i := 0; i < 256; i++ {
		input[i] = byte(i)
	}
	output := make([]byte, len(input)*2+4)
	expected := make([]byte, len(input)*2+4)
	ui8toi16(input, expected)
	Ui8toi16(input, output)
	if bytes.Compare(output, expected) != 0 {
		t.Fatalf("Output doesn't match expected: %+v != %+v", output, expected)
	}

	// Make sure unmatched input and output lengths don't cause a panic/segfault
	Ui8toi16([]byte{}, []byte{1, 2})
	Ui8toi16([]byte{1, 2}, []byte{1, 2})
}

func BenchmarkUi8toi16(b *testing.B) {
	input := make([]byte, 256)
	for i := 0; i < 256; i++ {
		input[i] = byte(i)
	}
	output := make([]byte, len(input)*2)
	for i := 0; i < b.N; i++ {
		Ui8toi16(input, output)
	}
}

func BenchmarkUi8toi16_Go(b *testing.B) {
	input := make([]byte, 256)
	for i := 0; i < 256; i++ {
		input[i] = byte(i)
	}
	output := make([]byte, len(input)*2)
	for i := 0; i < b.N; i++ {
		ui8toi16(input, output)
	}
}
