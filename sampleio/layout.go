package sampleio

import (
	"strconv"

	"samuelks.com/dsp/pcm"
)

// Container is how a stream's samples are packaged.
type Container int

const (
	// DetectContainer, the zero value, has a Reader read a WAV when the stream
	// starts with one and a headerless stream otherwise.
	DetectContainer Container = iota

	// WAV is a RIFF/WAVE or RF64 file. A Reader told to expect one fails on a
	// stream without the header.
	WAV

	// Raw is a headerless stream, which must be told its format and rate. A
	// Reader told to read one does so even when the stream has a header, which
	// is how to look at a WAV whose header is wrong.
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

// Layout is what a stream of samples looks like on disk. Options carries the
// one a Reader is told and Info the one it found.
type Layout struct {
	// Format is the sample encoding. A WAV header states its own and wins over
	// this one; a headerless stream requires it.
	Format pcm.Format

	// Rate is the sample rate in Hz. On a WAV it overrides the header, with a
	// Note; a headerless stream requires it.
	Rate float64

	// Channels is how many are interleaved, counting I and Q separately. Zero
	// selects 2 when IQ is set and 1 otherwise. A WAV header that disagrees is
	// an error.
	Channels int

	// IQ reads channel pairs as interleaved I/Q, so the stream is complex and
	// half as many channels wide as it looks.
	IQ bool

	// Container is how the samples are packaged. Info always reports WAV or
	// Raw.
	Container Container
}
