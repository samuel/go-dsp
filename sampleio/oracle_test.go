package sampleio

import (
	"flag"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// oracle runs the fixture headers past sox, which wrote them, and past ffprobe,
// which did not.
//
// It is off by default and skips when the binary is absent, the same form and
// the same rule as firpm's generate-and-compare: developer-run, never
// CI-blocking. Only the header fields are compared, never the numbers. A test
// asserting agreement with another tool's decibel convention would fail on that
// tool's next release, and it would be testing sox.
var oracle = flag.Bool("oracle", false, "cross-check the WAV fixtures against sox and ffprobe")

func TestHeadersAgreeWithSox(t *testing.T) {
	if !*oracle {
		t.Skip("run with -oracle to cross-check against sox and ffprobe")
	}
	sox, err := exec.LookPath("sox")
	if err != nil {
		t.Skip("sox is not installed")
	}
	files, err := filepath.Glob(filepath.Join("testdata", "*.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures to check")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			r, err := Open(path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := r.Close(); err != nil {
					t.Error(err)
				}
			}()
			info := r.Info()

			out, err := exec.Command(sox, "--i", path).Output()
			if err != nil {
				t.Fatalf("sox --i: %v", err)
			}
			fields := soxFields(string(out))
			for _, c := range []struct {
				name string
				got  int64
				key  string
			}{
				{"channels", int64(info.Channels), "Channels"},
				{"rate", int64(info.Rate), "Sample Rate"},
				{"frames", info.Frames, "samples"},
			} {
				want, ok := fields[c.key]
				if !ok {
					t.Errorf("sox did not report %s:\n%s", c.key, out)
					continue
				}
				if c.got != want {
					t.Errorf("%s: got %d, sox says %d", c.name, c.got, want)
				}
			}
			// Only for PCM. sox's Precision for a float format is how many
			// bits it thinks the encoding is good for -- 25 for float32, 54 for
			// float64 -- rather than the width of the container, so it is not
			// the same number and comparing them proves nothing.
			if bits, ok := fields["Precision"]; ok && info.Codec == "pcm" && int64(info.Bits) != bits {
				t.Errorf("bits: got %d, sox says %d", info.Bits, bits)
			}
		})
	}
}

var soxSamples = regexp.MustCompile(`=\s*(\d+)\s+samples`)

// soxFields pulls the numbers out of `sox --i`, which prints "Name : value"
// lines plus a Duration line carrying the frame count.
func soxFields(out string) map[string]int64 {
	f := make(map[string]int64)
	for line := range strings.SplitSeq(out, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if name == "Duration" {
			if m := soxSamples.FindStringSubmatch(value); m != nil {
				if n, err := strconv.ParseInt(m[1], 10, 64); err == nil {
					f["samples"] = n
				}
			}
			continue
		}
		// "24-bit" for Precision, a plain number for the rest.
		value, _, _ = strings.Cut(value, "-bit")
		if n, err := strconv.ParseInt(strings.Fields(value + " ")[0], 10, 64); err == nil {
			f[name] = n
		}
	}
	return f
}

func TestHeadersAgreeWithFFprobe(t *testing.T) {
	if !*oracle {
		t.Skip("run with -oracle to cross-check against sox and ffprobe")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	files, err := filepath.Glob(filepath.Join("testdata", "*.wav"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			r, err := Open(path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := r.Close(); err != nil {
					t.Error(err)
				}
			}()
			info := r.Info()

			out, err := exec.Command(ffprobe, "-v", "error",
				"-show_entries", "stream=sample_rate,channels",
				"-of", "default=noprint_wrappers=1", path).Output()
			if err != nil {
				t.Fatalf("ffprobe: %v", err)
			}
			got := map[string]int64{
				"sample_rate": int64(info.Rate),
				"channels":    int64(info.Channels),
			}
			for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
				if !ok {
					continue
				}
				want, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					continue
				}
				if mine, ok := got[key]; ok && mine != want {
					t.Errorf("%s: got %d, ffprobe says %d", key, mine, want)
				}
			}
		})
	}
}
