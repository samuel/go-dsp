package dsp

// Ui8toi16 converts and scales unsigned 8-bit samples to 16-bit signed samples.
func Ui8toi16(input []byte, output []int16)
func ui8toi16(input []byte, output []int16) {
	n := len(output)
	if len(input) < n {
		n = len(input)
	}
	for i, v := range input[:n] {
		v -= 128
		v16 := int16((uint16(v) << 8) | uint16(v))
		output[i] = v16
	}
}

// Ui8toi16b converts and scales unsigned 8-bit samples to 16-bit signed samples.
func Ui8toi16b(input, output []byte)
func ui8toi16b(input, output []byte) {
	n := len(output) / 2
	if len(input) < n {
		n = len(input)
	}
	for i, v := range input[:n] {
		v -= 128
		output[i*2] = v
		output[i*2+1] = v
	}
}

// Ui8tof32 converts unsigned 8-bit samples to 32-bit float.
// It does not scale the samples.
func Ui8tof32(input []byte, output []float32)
func ui8tof32(input []byte, output []float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	for i, v := range input[:n] {
		output[i] = float32(int(v) - 128)
	}
}

// I8tof32 converts signed 8-bit samples to 32-bit float.
// It does not scale the samples.
func I8tof32(input []byte, output []float32)
func i8tof32(input []byte, output []float32) {
	n := len(input)
	if len(output) < n {
		n = len(output)
	}
	for i, v := range input[:n] {
		output[i] = float32(int8(v))
	}
}

// Ui8toc64 converts unsigned 8-bit interleaved complex samples to 32-bit float.
// It does not scale the samples.
func Ui8toc64(input []byte, output []complex64)
func ui8toc64(input []byte, output []complex64) {
	n := len(input) / 2
	if len(output) < n {
		n = len(output)
	}
	for i := 0; i < n; i++ {
		output[i] = complex(
			float32(int(input[i*2])-128),
			float32(int(input[i*2+1])-128),
		)
	}
}

// F32toi16 converts scaled 32-bit floats to 16-bit integers.
func F32toi16(input []float32, output []int16, scale float32)
func f32toi16(input []float32, output []int16, scale float32) {
	n := len(output)
	if len(input) < n {
		n = len(input)
	}
	for i, v := range input[:n] {
		output[i] = int16(v * scale)
	}
}

// F32toi16ble converts float32 to int16 stored in a byte slice. The values
// are stored in little-endian.
func F32toi16ble(input []float32, output []byte, scale float32)
func f32toi16ble(input []float32, output []byte, scale float32) {
	n := len(output) / 2
	if len(input) < n {
		n = len(input)
	}
	for i, v := range input[:n] {
		v := uint16(int16(v * scale))
		output[i*2] = uint8(v & 0xff)
		output[i*2+1] = uint8(v >> 8)
	}
}

// I16bleToF64 converts int16 stored in a byte slice as little endian to float64.
func I16bleToF64(input []byte, output []float64, scale float64)
func i16bleToF64(input []byte, output []float64, scale float64) {
	n := len(input) / 2
	if len(output) < n {
		n = len(output)
	}
	for i := range output[:n] {
		output[i] = float64(int16(uint16(input[i*2])|(uint16(input[i*2+1])<<8))) * scale
	}
}

// I16bleToF32 converts int16 stored in a byte slice as little endian to float64.
func I16bleToF32(input []byte, output []float32, scale float32)
func i16bleToF32(input []byte, output []float32, scale float32) {
	n := len(input) / 2
	if len(output) < n {
		n = len(output)
	}
	for i := range output[:n] {
		output[i] = float32(int16(uint16(input[i*2])|(uint16(input[i*2+1])<<8))) * scale
	}
}
