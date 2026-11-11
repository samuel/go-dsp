package convert

// U8ToC64 converts unsigned 8-bit interleaved complex samples to complex64
// (32-bit parts), subtracting the 128 bias. It does not scale.
//
//go:noescape
func U8ToC64(dst []complex64, src []byte)
