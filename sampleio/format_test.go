package sampleio

import (
	"strings"
	"testing"

	"samuelks.com/dsp/convert"
)

// TestParseFormatSpec covers the names an SDR capture actually carries, where a
// leading c says the samples are interleaved I/Q.
func TestParseFormatSpec(t *testing.T) {
	for _, tc := range []struct {
		in     string
		want   convert.Format
		wantIQ bool
	}{
		{"cu8", convert.U8, true}, // rtl-sdr
		{"cs8", convert.I8, true}, // hackrf
		{"cs16le", convert.I16LE, true},
		{"cf32le", convert.F32LE, true},
		{"ci16le", convert.I16LE, true},
		{"u8", convert.U8, false},
		{"i16le", convert.I16LE, false},
		{"f32le", convert.F32LE, false},
	} {
		got, iq, err := ParseFormatSpec(tc.in)
		if err != nil {
			t.Errorf("ParseFormatSpec(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want || iq != tc.wantIQ {
			t.Errorf("ParseFormatSpec(%q) = (%v, %v), want (%v, %v)", tc.in, got, iq, tc.want, tc.wantIQ)
		}
	}

	for _, bad := range []string{"c", "cc", "cpcm", "cx16le"} {
		if f, iq, err := ParseFormatSpec(bad); err == nil {
			t.Errorf("ParseFormatSpec(%q) = (%v, %v), want an error", bad, f, iq)
		}
	}

	// The message has to name the string the caller passed, not the tail left
	// after the c was stripped.
	if _, _, err := ParseFormatSpec("cbogus"); err == nil || !strings.Contains(err.Error(), "cbogus") {
		t.Errorf("the error for cbogus was %v", err)
	}
}
