// +build gofuzz

package dsp

func Fuzz(data []byte) int {
	data = data[:len(data)/2*2]
	output := make([]float32, len(data)/2)
	expected := make([]float32, len(data)/2)
	I16bleToF32(data, output, 2.0)
	i16bleToF32(data, expected, 2.0)
	for i, v := range expected {
		if output[i] != v {
			return 0
		}
	}
	return 1
}
