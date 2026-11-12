package sampleio

import (
	"strconv"

	"samuelks.com/dsp/pcm"
)

// Container is how a stream's samples are packaged.
type Container int

const (
	// DetectContainer is the zero value. A Reader reads a WAV when the stream
	// starts with a WAV header and a headerless stream otherwise. A Writer
	// writes a WAV.
	DetectContainer Container = iota

	// WAV is a RIFF/WAVE or RF64 file. A Reader returns an error when the
	// stream does not start with the header.
	WAV

	// Raw is a headerless stream, so the Layout must give its format and rate.
	// A Reader does not look for a header, which makes this the way to read a
	// WAV whose header is wrong.
	Raw
)

// String returns "detect", "wav" or "raw".
func (c Container) String() string {
	switch c {
	case DetectContainer:
		return "detect"
	case WAV:
		return "wav"
	case Raw:
		return "raw"
	}
	return "Container(" + strconv.Itoa(int(c)) + ")"
}

// Layout describes a stream of samples: how they are encoded, how fast, how
// many channels, and what container holds them. Open and NewReader take the
// layout of a headerless stream, Create and NewWriter take the layout to
// write, and Info reports the layout in use. A Reader's Info().Layout given to
// a Writer writes the same shape back out.
type Layout struct {
	// Format is the sample encoding. A headerless stream and a Writer require
	// it. A Reader ignores it for a WAV, whose header states its own.
	Format pcm.Format

	// Rate is the sample rate in Hz. A headerless stream read and a WAV
	// written require it. Given to a Reader of a WAV, it replaces the header's
	// rate, with a Note.
	Rate float64

	// Channels is the number of interleaved channels, with I and Q counted as
	// two. Zero means 2 when IQ is set and 1 otherwise. A Reader returns an
	// error when a WAV header says a different number.
	Channels int

	// IQ treats each pair of channels as one complex signal, I then Q, so the
	// stream is read and written as complex samples. A Reader accepts any even
	// number of channels; a Writer takes exactly two.
	IQ bool

	// Container is how the samples are packaged. The zero value detects it on
	// read and selects WAV on write. Info always reports WAV or Raw.
	Container Container

	// ChannelMask assigns the channels of a WAV to speakers, one Speaker bit
	// per channel in channel order. A Reader reports the header's mask, or
	// zero when it has none. A Writer given zero writes no mask, or the usual
	// mono or stereo mask where the format needs an extensible header. A
	// headerless stream does not store it.
	ChannelMask uint32
}
