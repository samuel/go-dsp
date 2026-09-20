// Package encoding converts between sample encodings: the raw integer and byte
// formats an SDR or audio source delivers, float, and interleaved complex.
//
// The conversions are named <source>To<destination>: U8 and I8 for 8-bit
// samples, I16, I24 and I32 for wider integers, F32 and F64 for floats, C64 for
// interleaved complex64, and an LE suffix on whichever side is a byte slice
// holding little-endian values.
//
// None of them scale; each integer form keeps its own full-scale range, and the
// normalizing factor belongs in the coefficients of the next linear stage,
// where it costs nothing. dsp.VScale is the fallback when nothing can absorb
// it. F32ToI16 is the exception: samples past full scale clip to the rails
// rather than wrap and NaN converts to zero, so the gain behind it stays in
// whatever produced the samples.
//
// Every conversion moves min(len(src), len(dst)) samples between separate dst
// and src slices and tolerates unaligned slices. The per-width paths are the
// generated and hand-written assembly for 386, amd64, arm and arm64, with a
// scalar implementation on every other architecture.
package encoding
