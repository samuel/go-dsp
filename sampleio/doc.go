// Package sampleio reads and writes audio and I/Q sample files.
//
// It covers the two forms signal captures arrive in: a WAV file, which carries
// its own rate and encoding, and a headerless stream of samples, which carries
// nothing and must be told both. Either way a Reader hands over ready-scaled
// samples, so nothing downstream knows about interleaving, byte order or bit
// depth.
//
// # Reading
//
// A Reader hands over samples through two generic methods: ReadSamples for one
// selected channel or I/Q pair, and ReadPlanar for every channel or pair at
// once into one slice each. The width is the type of dst, and each read
// decodes straight to it, so a float32 or complex64 read costs no float64
// detour. Neither is called Read: they have io.Reader's shape but not its
// contract, since the unit is a sample of one channel rather than a byte.
//
// The planar read is not a convenience over the other. It decodes the
// interleaved run once and splits it in one pass instead of gathering each
// channel a frame at a time, which is 3.4x a Reader per channel for stereo and
// 2.7x at eight channels.
//
// A stream's shape is a Layout -- format, rate, channels, I/Q and container --
// which Options carries in and Info carries out.
//
// # Writing
//
// A Writer is the mirror of a Reader: WriteSamples takes one channel or I/Q
// pair, WritePlanar one slice per channel, at any width, and the Writer
// encodes, interleaves and wraps them in the container its Layout names. A
// Reader's Info().Layout given to a Writer writes the same shape back out.
//
// A WAV written to a file that can be sought has its sizes filled in by Close,
// and becomes RF64 past 4 GiB. One written down a pipe carries the
// placeholder sizes a streaming reader expects and runs to the end of the
// stream. An integer sample past full scale is clipped, with a Note in Info
// the first time it happens.
//
// # Formats
//
// A sample encoding is a pcm.Format, and samples are decoded by pcm.Decode,
// which normalizes them to full scale: the most negative integer code is
// exactly -1, so a full-scale sine reads
// exactly 0 dBFS, which every decibel measured downstream is relative to.
// Float samples are passed through unscaled, since a float file is already
// written in those units; a Note records it if any sample is outside the range.
//
// Complex is not a format. Interleaved I/Q is a layout over an ordinary element
// type rather than an encoding of its own -- pcm.Decode into a complex slice
// reads interleaved pairs of any format -- so it is Layout.IQ, and
// ParseFormatSpec reads the leading c of the names an SDR writes: cu8 is
// rtl-sdr's format, pcm.U8 with IQ set.
//
// # Byte order
//
// WAV is a RIFF container and always little-endian, so the big-endian formats
// only ever come from a raw stream that is explicitly said to be one.
//
//go:generate go run testdata/generate.go
package sampleio
