// Package sampleio reads audio and I/Q sample files.
//
// It covers the two forms signal captures arrive in: a WAV file, which carries
// its own rate and encoding, and a headerless stream of samples, which carries
// nothing and must be told both. Either way a Reader hands over ready-scaled
// samples, so nothing downstream knows about interleaving, byte order or bit
// depth.
//
// # Reading
//
// A Reader hands over samples through one method per width and layout:
// ReadFloat64 and ReadFloat32 for one selected channel, ReadFloat64Planar and
// ReadFloat32Planar for every channel at once into one slice each, and
// ReadComplex128 for an I/Q pair. None of them is called Read: they have
// io.Reader's method set but not its contract, since the unit is a sample of one
// channel rather than a byte. The width is in the name because every read
// carries one, and a caller picks a method rather than recalling which width
// the unqualified name meant.
//
// The planar reads are not a convenience over the others. They decode the
// interleaved run once and split it in one pass instead of gathering each
// channel a frame at a time, which is 3.1x a Reader per channel for stereo and
// 2.5x at eight channels.
//
// # Format names
//
// The names are dsp's, so that a format reads as the conversion it is: U8 and
// I8 for 8-bit samples, I16, I24 and I32 for wider integers, F32 and F64 for
// floats, with LE or BE for the byte order of anything wider than a byte.
// ParseFormat also accepts the ffmpeg spelling of the signed integers, where
// s16le means the same as i16le.
//
// Complex is not one of them. Interleaved I/Q is a layout over an ordinary
// element type rather than an encoding of its own -- which is what dsp already
// means by C64, an interleaved pair of F32 -- so it is Options.IQ, and
// ParseFormatSpec reads the leading c of the names an SDR writes: cu8 is
// rtl-sdr's format, U8 with IQ set.
//
// # Full scale
//
// An integer sample is divided by 2^(bits-1), so the most negative code is
// exactly -1 and the most positive is one step short of +1. That is what makes
// a full-scale sine read exactly 0 dBFS, which every decibel measured
// downstream is relative to. Float samples are passed through unscaled, since a
// float file is already written in those units; a Note records it if any sample
// is outside the range.
//
// # Byte order
//
// WAV is a RIFF container and always little-endian, so the big-endian formats
// only ever come from a raw stream that is explicitly said to be one.
//
//go:generate go run testdata/generate.go
package sampleio
