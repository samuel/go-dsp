//go:build !arm && !amd64 && !arm64

package encoding

// U8ToF32 converts unsigned 8-bit samples to float32, subtracting the 128 bias.
// It does not scale: the range is -128 to 127.
func U8ToF32(dst []float32, src []byte) {
	u8ToF32(dst, src)
}
