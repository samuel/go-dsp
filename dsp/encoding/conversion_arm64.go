package encoding

// U8ToC64 converts unsigned 8-bit interleaved complex samples to complex64
// (32-bit parts), subtracting the 128 bias. It does not scale.
func U8ToC64(dst []complex64, src []byte)
