package sdr

func Ui8toi16b(input []byte, output []byte)

func ui8toi16b(input []byte, output []byte) {
	n := len(output) / 2
	if len(input) < n {
		n = len(input)
	}
	for i := 0; i < n; i++ {
		v := input[i] - 128
		output[i*2] = v
		output[i*2+1] = v
	}
}

var F32toi16b = f32toi16b

func f32toi16b(input []float32, output []byte, scale float32) {
	for i := 0; i < len(input); i++ {
		v := int16(input[i] * scale)
		output[i*2] = uint8(uint16(v) & 0xff)
		output[i*2+1] = uint8(uint16(v) >> 8)
	}
}

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
