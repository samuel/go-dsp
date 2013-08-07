package main

func Ui8toi16(input []byte, output []byte)

func ui8toi16(input []byte, output []byte) {
	n := len(input)
	n2 := len(output) / 2
	if n2 < n {
		n = n2
	}
	for i := 0; i < n; i++ {
		v := input[i] - 128
		output[i*2] = v
		output[i*2+1] = v
	}
}
