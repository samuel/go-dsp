package sampleio

import (
	"fmt"
	"strings"
)

// Format is a sample encoding: an element type, a width and a byte order.
//
// See the package comment for the naming and for the full-scale convention.
type Format int

// The encodings a sample file can hold. The zero value is not one of them, so a
// Format left unset is a reportable mistake rather than a silent u8.
const (
	U8 Format = iota + 1
	I8
	I16LE
	I16BE
	I24LE
	I24BE
	I32LE
	I32BE
	F32LE
	F32BE
	F64LE
	F64BE
)

// info is the one description of a format. It is a switch rather than a
// package-level table, which an importer could reorder.
//
// bits is the significant bits of an integer sample, zero for a float, which is
// already in full-scale units.
func (f Format) info() (name, desc string, width, bits int) {
	switch f {
	case U8:
		return "u8", "8-bit unsigned", 1, 8
	case I8:
		return "i8", "8-bit signed", 1, 8
	case I16LE:
		return "i16le", "16-bit signed, little-endian", 2, 16
	case I16BE:
		return "i16be", "16-bit signed, big-endian", 2, 16
	case I24LE:
		return "i24le", "24-bit signed, little-endian", 3, 24
	case I24BE:
		return "i24be", "24-bit signed, big-endian", 3, 24
	case I32LE:
		return "i32le", "32-bit signed, little-endian", 4, 32
	case I32BE:
		return "i32be", "32-bit signed, big-endian", 4, 32
	case F32LE:
		return "f32le", "32-bit float, little-endian", 4, 0
	case F32BE:
		return "f32be", "32-bit float, big-endian", 4, 0
	case F64LE:
		return "f64le", "64-bit float, little-endian", 8, 0
	case F64BE:
		return "f64be", "64-bit float, big-endian", 8, 0
	}
	return "", "", 0, 0
}

// String returns the name a flag takes, such as "i16le".
func (f Format) String() string {
	name, _, _, _ := f.info()
	if name == "" {
		return fmt.Sprintf("Format(%d)", int(f))
	}
	return name
}

// Desc returns a phrase describing the encoding, for a usage message.
func (f Format) Desc() string {
	_, desc, _, _ := f.info()
	return desc
}

// Width returns the size of one sample in bytes: 3 for the 24-bit formats,
// which are packed rather than padded out to four.
func (f Format) Width() int {
	_, _, w, _ := f.info()
	return w
}

// Valid reports whether f is one of the encodings this package knows.
func (f Format) Valid() bool { return f.Width() != 0 }

// Formats returns every encoding, in a fresh slice, in declaration order.
func Formats() []Format {
	return []Format{
		U8, I8,
		I16LE, I16BE,
		I24LE, I24BE,
		I32LE, I32BE,
		F32LE, F32BE,
		F64LE, F64BE,
	}
}

// FormatNames returns the name of every encoding, in a fresh slice.
func FormatNames() []string {
	f := Formats()
	names := make([]string, len(f))
	for i, v := range f {
		names[i] = v.String()
	}
	return names
}

// ParseFormat parses an encoding name, accepting ffmpeg's s16le spelling of a
// signed integer too; String always reports the i form.
func ParseFormat(s string) (Format, error) {
	name := strings.ToLower(strings.TrimSpace(s))

	// ffmpeg says s16le where dsp says i16le. Everything else agrees.
	if rest, ok := strings.CutPrefix(name, "s"); ok {
		name = "i" + rest
	}
	for _, f := range Formats() {
		if f.String() == name {
			return f, nil
		}
	}
	return 0, fmt.Errorf("sampleio: unknown sample format %q, want one of %s",
		s, strings.Join(FormatNames(), ", "))
}

// ParseFormatSpec parses an encoding name that may carry a leading c for
// interleaved I/Q samples, which is how an SDR capture is usually named: cu8 is
// rtl-sdr's unsigned bytes, cs16le a 16-bit I/Q recording. It is what a command
// line flag should take, so one flag says both what the samples are and how
// they are laid out.
func ParseFormatSpec(s string) (f Format, iq bool, err error) {
	name := strings.ToLower(strings.TrimSpace(s))
	if rest, ok := strings.CutPrefix(name, "c"); ok {
		iq = true
		name = rest
	}
	f, err = ParseFormat(name)
	if err != nil {
		// Report the string the caller passed, not the trimmed tail.
		if iq {
			return 0, false, fmt.Errorf("sampleio: unknown sample format %q, want one of %s, "+
				"optionally with a leading c for interleaved complex", s, strings.Join(FormatNames(), ", "))
		}
		return 0, false, err
	}
	return f, iq, nil
}
