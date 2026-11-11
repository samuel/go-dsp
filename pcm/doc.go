// Package pcm encodes and decodes samples: the raw integer and float formats an
// SDR or audio source delivers or a file stores, to and from full-scale float
// and complex samples.
//
// Format names an encoding: U8 and I8 for 8-bit samples, I16, I24 and I32 for
// wider signed integers, F32 and F64 for floats, with LE or BE for the byte
// order of anything wider than a byte. ParseFormat also accepts the ffmpeg
// spelling of the signed integers, where s16le means the same as i16le.
//
// Decode turns bytes into samples of any dsp.Sample type and Encode turns them
// back. A complex sample is an interleaved I/Q pair of the format's element.
// Samples are at full scale on both sides: an integer is divided by
// 2^(bits-1) on the way in, so a full-scale sine reads 0 dBFS, and multiplied
// by it on the way out. The factor is a power of two, so it costs no accuracy
// and gives the same answer on every architecture.
//
// # Rounding and clipping
//
// Encode rounds to the nearest integer code, ties to even, with no dither. A
// caller that wants dither adds it before Encode. A sample past full scale
// clips to the format's rails, NaN encodes as zero, and nothing reports either;
// a writer that wants to know looks at the samples first.
//
// # U8
//
// U8 is offset binary with its bias of 128 removed, so the codes round-trip
// exactly. A source whose midpoint is 127.5, as an rtl-sdr's is, therefore
// decodes with a DC offset of half a code, which dsp.DCFilter removes.
//
// # Implementation
//
// Every encoding has one little-endian kernel per float width, and a
// big-endian format reverses its bytes in blocks through a buffer on the stack
// around that kernel. The kernels a capture spends its time in -- U8, I8 and
// I16LE into float32, and float32 into I16LE -- are assembly or archsimd on
// 386, amd64, arm and arm64; every other kernel, and every kernel elsewhere, is
// Go. All of them agree bit for bit with the Go reference that defines them.
// Slices may be unaligned, and src and dst must not overlap.
package pcm
