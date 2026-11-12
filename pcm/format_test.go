package pcm

import (
	"slices"
	"strings"
	"testing"
)

func TestFormatNamesRoundTrip(t *testing.T) {
	for _, f := range Formats() {
		name := f.String()
		got, err := ParseFormat(name)
		if err != nil {
			t.Errorf("ParseFormat(%q): %v", name, err)
			continue
		}
		if got != f {
			t.Errorf("ParseFormat(%q) = %v, want %v", name, got, f)
		}
		if f.Desc() == "" {
			t.Errorf("%v has no description", f)
		}
		if f.Width() <= 0 {
			t.Errorf("%v has width %d", f, f.Width())
		}
		if !f.Valid() {
			t.Errorf("%v reports itself invalid", f)
		}
	}
}

func TestFormatZeroValue(t *testing.T) {
	var f Format
	if f.Valid() {
		t.Error("the zero Format reports itself valid")
	}
	if f.Width() != 0 {
		t.Errorf("the zero Format has width %d", f.Width())
	}
	// It must still print something a message can use.
	if s := f.String(); !strings.Contains(s, "0") {
		t.Errorf("the zero Format prints %q", s)
	}
}

func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Format
	}{
		{"i16le", I16LE},
		{"I16LE", I16LE},
		{"  i16le  ", I16LE},
		{"u8", U8},
		{"i8", I8},
		{"f64be", F64BE},

		// The ffmpeg spelling of a signed integer.
		{"s16le", I16LE},
		{"s24le", I24LE},
		{"s32be", I32BE},
		{"s8", I8},

		{"alaw", ALaw},
		{"mulaw", MuLaw},
		{"ulaw", MuLaw},
	} {
		got, err := ParseFormat(tc.in)
		if err != nil {
			t.Errorf("ParseFormat(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseFormat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"", "i16", "le16s", "8uc", "i12le", "pcm", "s", "c", "i16LEE", "law", "alawle", "salaw"} {
		if got, err := ParseFormat(bad); err == nil {
			t.Errorf("ParseFormat(%q) = %v, want an error", bad, got)
		}
	}
}

// TestFormatsFresh is the no-shared-table rule: a caller that reorders what it
// is given must not reorder it for everyone else.
func TestFormatsFresh(t *testing.T) {
	a, b := Formats(), Formats()
	if !slices.Equal(a, b) {
		t.Fatal("Formats disagrees with itself")
	}
	slices.Reverse(a)
	if slices.Equal(a, Formats()) {
		t.Error("reversing the result of Formats changed what it returns")
	}

	n1, n2 := FormatNames(), FormatNames()
	if !slices.Equal(n1, n2) {
		t.Fatal("FormatNames disagrees with itself")
	}
	n1[0] = "clobbered"
	if FormatNames()[0] == "clobbered" {
		t.Error("writing to the result of FormatNames changed what it returns")
	}
	if len(n1) != len(Formats()) {
		t.Errorf("FormatNames has %d entries and Formats has %d", len(n1), len(Formats()))
	}
}
