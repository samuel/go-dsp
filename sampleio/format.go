package sampleio

import (
	"fmt"
	"strings"

	"samuelks.com/dsp/pcm"
)

// ParseFormatSpec parses the name of a sample format, as pcm.ParseFormat does,
// and also accepts a leading c meaning interleaved I/Q. SDR captures are
// usually named this way: cu8 is rtl-sdr's unsigned bytes and cs16le is 16-bit
// I/Q. It lets one command line flag give both Layout.Format and Layout.IQ.
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
