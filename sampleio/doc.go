// Package sampleio reads and writes files of audio and I/Q samples.
//
// It handles two kinds of file:
//
//   - A WAV file (RIFF/WAVE or RF64), whose header gives the sample format,
//     rate and channel count.
//   - A headerless stream of raw samples, such as an SDR capture. Nothing in
//     the file describes it, so the caller supplies the format and rate.
//
// Samples are float32, float64, complex64 or complex128, scaled so that full
// scale is 1. The package takes care of byte order, bit depth, interleaving
// and the WAV header.
//
// # Layout
//
// A Layout describes a stream: its format, rate, channel count, whether it is
// I/Q, and its container. The same struct is used in three places:
//
//   - Open and NewReader take the layout of a headerless stream. For a WAV the
//     zero Layout is enough, since the header supplies the rest.
//   - Create and NewWriter take the layout to write.
//   - Info reports the layout in use, along with the length and any notes.
//
// A Reader's Info().Layout given to a Writer writes the same shape back out.
//
// # Reading
//
// A frame is one sample of every channel. A Reader has two read methods, both
// generic over the sample type:
//
//   - ReadSamples reads a stream that holds one signal: a single channel, or a
//     single I/Q pair.
//   - ReadPlanar reads every channel at once, into one slice per channel. A
//     nil slice skips that channel, which is how to read some channels of a
//     file and not others.
//
// Samples are decoded directly to the type of the destination slice, so
// reading float32 does not go through float64. A real stream is read into
// float32 or float64 and an I/Q stream into complex64 or complex128.
//
// Reading part of a stream is done with the methods, not with options: Skip
// moves forward a number of frames on any source, SeekFrame moves to a frame
// of a source that can seek, and a read stops when the destination slice is
// full. Combining channels is left to the caller, who reads them with
// ReadPlanar and mixes them as needed.
//
// For a WAV, Chunks returns the chunks that come before the samples, such as
// LIST, bext and cue, and Info().ChannelMask is the speaker assignment.
//
// # Writing
//
// A Writer mirrors a Reader. WriteSamples writes a stream of one channel or
// one I/Q pair, and WritePlanar writes one slice per channel. Either takes any
// of the four sample types.
//
// A WAV header states the size of the file, which is not known until the last
// sample is written. When the destination can seek, as a file can, Close goes
// back and fills the sizes in, and makes the file RF64 if it is larger than
// 4 GiB. When it cannot, as with a pipe, the header keeps placeholder sizes
// that tell a reader to read to the end of the stream.
//
// # Formats and scaling
//
// A sample encoding is a pcm.Format.
//
// Integer samples are divided by their full scale on read, so the most
// negative code is exactly -1 and a full-scale sine measures 0 dBFS. On write
// they are multiplied back, rounded and clipped to the format's range. The
// first clipped sample adds a Note to the Writer's Info.
//
// Float samples are read and written unchanged. Nothing keeps a float file
// within full scale, so the first sample read outside it adds a Note to the
// Reader's Info.
//
// G.711 is pcm.ALaw and pcm.MuLaw, in a WAV or as a headerless stream. Its
// levels are those of a 16-bit file, so converting G.711 to pcm.I16LE loses
// nothing. Its highest level is a little below full scale.
//
// I/Q is not a format of its own. An I/Q stream is two interleaved channels,
// I then Q, in any format, so it is selected by Layout.IQ. SDR tools name
// these captures with a leading c, such as cu8 for rtl-sdr's unsigned bytes,
// and ParseFormatSpec turns such a name into a format and the IQ flag.
//
// A WAV is always little-endian. The big-endian formats can only be read from
// or written to a headerless stream.
//
// The WAV container itself is read and written through package riff.
//
//go:generate go run testdata/generate.go
package sampleio
