package pcm

import (
	"fmt"
	"strings"
)

// Format is a sample encoding: the type of each sample, its width and its
// byte order. The package comment lists them.
type Format int

// The encodings. The zero Format is not valid, so a Format that was never set
// is caught and is not taken for U8.
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
	ALaw
	MuLaw
)

// info returns everything the package knows about a format. A format that is
// not valid returns zero values.
//
// bits is the number of bits in an integer sample, 8 for a G.711 code, and
// zero for a float, which is not scaled.
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
	case ALaw:
		return "alaw", "8-bit G.711 A-law", 1, 8
	case MuLaw:
		return "mulaw", "8-bit G.711 mu-law", 1, 8
	}
	return "", "", 0, 0
}

// String returns the format's name, such as "i16le". ParseFormat reads it
// back.
func (f Format) String() string {
	name, _, _, _ := f.info()
	if name == "" {
		return fmt.Sprintf("Format(%d)", int(f))
	}
	return name
}

// Desc returns a short description of the format, such as "16-bit signed,
// little-endian", for a usage message.
func (f Format) Desc() string {
	_, desc, _, _ := f.info()
	return desc
}

// Width returns the size of one sample in bytes. A 24-bit format is 3 bytes;
// it is not padded to 4.
func (f Format) Width() int {
	_, _, w, _ := f.info()
	return w
}

// Bits returns the number of bits in a sample of an integer format. Decode
// divides by 2^(Bits-1) and Encode multiplies by it. A float format returns
// zero, since it is not scaled. A G.711 format returns 8, the size of its
// code, though its scale is that of 16 bits.
func (f Format) Bits() int {
	_, _, _, b := f.info()
	return b
}

// Float reports whether f holds floating-point samples.
func (f Format) Float() bool { return f.Valid() && f.Bits() == 0 }

// Companded reports whether f is ALaw or MuLaw, the G.711 formats, whose
// levels are not evenly spaced.
func (f Format) Companded() bool { return f == ALaw || f == MuLaw }

// Valid reports whether f is one of the defined formats.
func (f Format) Valid() bool { return f.Width() != 0 }

// Formats returns every format, in the order they are declared. The caller
// may modify the slice.
func Formats() []Format {
	return []Format{
		U8, I8,
		I16LE, I16BE,
		I24LE, I24BE,
		I32LE, I32BE,
		F32LE, F32BE,
		F64LE, F64BE,
		ALaw, MuLaw,
	}
}

// FormatNames returns the name of every format, in the same order as
// Formats. The caller may modify the slice.
func FormatNames() []string {
	f := Formats()
	names := make([]string, len(f))
	for i, v := range f {
		names[i] = v.String()
	}
	return names
}

// ParseFormat parses the name of a format, ignoring case and surrounding
// space. Besides the names String returns, it accepts ffmpeg's s prefix for a
// signed integer, as in "s16le", and "ulaw" for "mulaw".
func ParseFormat(s string) (Format, error) {
	name := strings.ToLower(strings.TrimSpace(s))

	// ffmpeg writes s16le for what this package calls i16le.
	if rest, ok := strings.CutPrefix(name, "s"); ok {
		name = "i" + rest
	}
	if name == "ulaw" {
		name = "mulaw"
	}
	for _, f := range Formats() {
		if f.String() == name {
			return f, nil
		}
	}
	return 0, fmt.Errorf("pcm: unknown sample format %q, want one of %s",
		s, strings.Join(FormatNames(), ", "))
}
