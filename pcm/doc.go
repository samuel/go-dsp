// Package pcm converts between raw sample bytes and full-scale float or
// complex samples.
//
// The bytes are in one of the encodings an SDR, a sound card or a file uses:
// 8-bit to 32-bit integers, 32-bit and 64-bit floats, and the G.711 A-law and
// mu-law codes. The samples are any dsp.Sample type: float32, float64,
// complex64 or complex128.
//
// # Formats
//
// A Format specifies an encoding:
//
//   - U8 and I8 are unsigned and signed 8-bit samples.
//   - I16, I24 and I32 are signed integers. I24 is three bytes, not padded to
//     four.
//   - F32 and F64 are floats.
//   - ALaw and MuLaw are the 8-bit codes of ITU-T G.711.
//
// A format wider than one byte ends in LE or BE for its byte order, as in
// I16LE. ParseFormat handles the names String returns, such as "i16le", and also
// accepts ffmpeg's spelling of the signed integers, such as "s16le".
//
// # Decode and Encode
//
// Decode turns bytes into samples, and Encode turns samples into bytes. Both
// handle as many whole samples as the shorter of the two slices holds and
// return that count. The slices may be unaligned, and must not overlap.
//
// A complex sample is two values of the format, I then Q, so a complex slice
// reads or writes interleaved I/Q.
//
// # Scaling
//
// Samples are at full scale: an integer format covers the range from -1 to
// just under 1. Decode divides an integer by 2^(bits-1), so the most negative
// code is exactly -1 and a full-scale sine measures 0 dBFS. Encode multiplies
// by the same factor. The factor is a power of two, so scaling loses no
// accuracy and gives the same result on every architecture.
//
// Float formats are not scaled. Decode and Encode only convert between the
// format's width and the sample's.
//
// # Rounding and clipping
//
// Encode rounds to the nearest integer code, with ties going to the even
// code. It adds no dither; a caller that wants dither adds it to the samples
// first.
//
// A sample outside full scale is clipped to the largest or smallest code of
// the format, and NaN is encoded as zero. Encode does not report either. A
// caller that needs to know checks the samples before encoding.
//
// # U8
//
// A U8 code is the sample plus 128, so code 128 is zero, and every code
// survives a Decode and an Encode unchanged.
//
// Some sources center their signal between two codes. An rtl-sdr's midpoint is
// 127.5, so its samples decode with a DC offset of half a code. dsp.DCFilter
// removes it.
//
// # G.711
//
// ALaw and MuLaw store each sample in 8 bits on a logarithmic scale. The 256
// levels are close together near zero and far apart at large amplitudes.
//
// Decode returns each code's level on the 16-bit scale, divided by 2^15. This
// is exact at either float width, and it matches other G.711 decoders, so
// converting G.711 to a 16-bit format loses nothing. The highest level is
// 32256/32768 for A-law and 32124/32768 for mu-law. A tone at the top of the
// G.711 range therefore measures about -0.14 dBFS for A-law and -0.17 dBFS
// for mu-law.
//
// Encode rounds the sample to 16 bits, as it does for I16, and picks the code
// whose step contains the result. Each level is the center of its step, and
// the boundaries between steps are G.711's decision values. A sample above the
// top step is clipped to the top level.
//
// Encode does not match the ITU's reference encoder, G.191, byte for byte.
// G.191 discards the low two or three bits of the 16-bit sample before
// encoding and takes the magnitude of a negative sample as its ones'
// complement. Encode keeps those bits.
//
// Every code survives a Decode and an Encode except mu-law's negative zero,
// 0x7f, which comes back as positive zero, 0xff. A-law has no zero level, so a
// zero sample is encoded as its smallest positive level.
//
// # Implementation
//
// Each encoding has one little-endian kernel per float width. A big-endian
// format is handled by reversing the bytes of each sample, a block at a time
// through a buffer on the stack, before or after that kernel.
//
// The kernels that captures use most are U8, I8 and I16LE into float32, and
// float32 into I16LE. On 386, amd64, arm and arm64 these are assembly, or
// archsimd under GOEXPERIMENT=simd. Every other kernel, and every kernel on
// other architectures, is Go. Each assembly kernel gives the same result, bit
// for bit, as the Go reference that defines it.
package pcm
