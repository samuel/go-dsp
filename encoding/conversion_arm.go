package encoding

// U8ToF32 converts unsigned 8-bit samples to float32, subtracting the 128 bias.
// It does not scale: the range is -128 to 127.
func U8ToF32(dst []float32, src []byte)

// U8ToC64 converts unsigned 8-bit interleaved complex samples to complex64
// (32-bit parts), subtracting the 128 bias. It does not scale.
func U8ToC64(dst []complex64, src []byte)
