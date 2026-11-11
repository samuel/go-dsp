package pcm

import (
	"testing"

	"samuelks.com/dsp/internal/kernelset"
)

// TestKernelSetComplete requires every build-tag group of the decode and
// encode families to declare the whole grid with the same signatures.
func TestKernelSetComplete(t *testing.T) {
	var decode, encode []string
	for _, k := range decodeF32Kernels {
		decode = append(decode, k.name)
	}
	for _, k := range decodeF64Kernels {
		decode = append(decode, k.name)
	}
	for _, k := range encodeF32Kernels {
		encode = append(encode, k.name)
	}
	for _, k := range encodeF64Kernels {
		encode = append(encode, k.name)
	}
	t.Run("decode", func(t *testing.T) { kernelset.Check(t, "decode_*.go", decode) })
	t.Run("encode", func(t *testing.T) { kernelset.Check(t, "encode_*.go", encode) })
}
