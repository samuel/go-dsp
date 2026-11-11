package sampleio

import (
	"fmt"
	"strings"

	"samuelks.com/dsp/pcm"
)

// ParseFormatSpec parses an encoding name that may carry a leading c for
// interleaved I/Q samples, which is how an SDR capture is usually named: cu8 is
// rtl-sdr's unsigned bytes, cs16le a 16-bit I/Q recording. It is what a command
// line flag should take, so one flag says both what the samples are and how
// they are laid out.
func ParseFormatSpec(s string) (f pcm.Format, iq bool, err error) {
	name := strings.ToLower(strings.TrimSpace(s))
	if rest, ok := strings.CutPrefix(name, "c"); ok {
		iq = true
		name = rest
	}
	f, err = pcm.ParseFormat(name)
	if err != nil {
		// Report the string the caller passed, not the trimmed tail.
		if iq {
			return 0, false, fmt.Errorf("sampleio: unknown sample format %q, want one of %s, "+
				"optionally with a leading c for interleaved complex", s, strings.Join(pcm.FormatNames(), ", "))
		}
		return 0, false, err
	}
	return f, iq, nil
}
