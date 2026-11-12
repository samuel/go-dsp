package sampleio

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"samuelks.com/dsp/pcm"
)

// oracle runs the fixture headers past sox, which wrote them, and past ffprobe,
// which did not, and every header the Writer produces past both.
//
// It is off by default and skips when the binary is absent, the same form and
// the same rule as firpm's generate-and-compare: developer-run, never
// CI-blocking. Only the header fields are compared, never the numbers. A test
// asserting agreement with another tool's decibel convention would fail on that
// tool's next release, and it would be testing sox.
var oracle = flag.Bool("oracle", false, "cross-check the WAV fixtures and the Writer against sox and ffprobe")

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
			r, err := Open(path, Layout{})
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
			// the same number and comparing them proves nothing. For G.711 it
			// is the 13 or 14 bits the code expands to.
			if bits, ok := fields["Precision"]; ok && !info.Format.Float() && !info.Format.Companded() && int64(info.Bits) != bits {
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
			r, err := Open(path, Layout{})
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

// writerFile is a WAV the Writer produced for the oracle, with what it holds.
type writerFile struct {
	path     string
	channels int
	rate     int64
	frames   int64 // -1 for the pipe form, whose sizes are placeholders
}

// writeOracleFiles writes every WAV format at one, two and three channels, in
// the form a file gets and the form a pipe gets.
func writeOracleFiles(t *testing.T) []writerFile {
	t.Helper()
	dir := t.TempDir()
	var files []writerFile
	for _, f := range []pcm.Format{pcm.U8, pcm.I16LE, pcm.I24LE, pcm.I32LE, pcm.F32LE, pcm.F64LE, pcm.ALaw, pcm.MuLaw} {
		for _, channels := range []int{1, 2, 3} {
			for _, seek := range []bool{true, false} {
				l := Layout{Format: f, Rate: 22050, Channels: channels, Container: WAV}
				const frames = 333
				planes := narrowPlanes[float64](testSignal(channels, frames))
				b := writeAll(t, l, seek, func(w *Writer) (int, error) {
					if channels == 1 {
						return w.WriteSamples(planes[0])
					}
					return w.WritePlanar(planes)
				})
				form, n := "file", int64(frames)
				if !seek {
					form, n = "pipe", -1
				}
				path := filepath.Join(dir, fmt.Sprintf("%v-%dch-%s.wav", f, channels, form))
				if err := os.WriteFile(path, b, 0o600); err != nil {
					t.Fatal(err)
				}
				files = append(files, writerFile{path, channels, 22050, n})
			}
		}
	}
	return files
}

// TestWriterAgreesWithSox checks that sox reads every header the Writer
// produces as the layout it was written with, and decodes the samples.
func TestWriterAgreesWithSox(t *testing.T) {
	if !*oracle {
		t.Skip("run with -oracle to cross-check against sox and ffprobe")
	}
	sox, err := exec.LookPath("sox")
	if err != nil {
		t.Skip("sox is not installed")
	}
	for _, wf := range writeOracleFiles(t) {
		t.Run(filepath.Base(wf.path), func(t *testing.T) {
			out, err := exec.Command(sox, "--i", wf.path).CombinedOutput()
			if err != nil {
				t.Fatalf("sox --i: %v\n%s", err, out)
			}
			fields := soxFields(string(out))
			if fields["Channels"] != int64(wf.channels) || fields["Sample Rate"] != wf.rate {
				t.Errorf("sox reads %d channels at %d Hz, want %d at %d:\n%s",
					fields["Channels"], fields["Sample Rate"], wf.channels, wf.rate, out)
			}
			if wf.frames >= 0 && fields["samples"] != wf.frames {
				t.Errorf("sox counts %d frames, want %d", fields["samples"], wf.frames)
			}
			// Decoding to nowhere fails on a header sox accepts but cannot
			// read samples through.
			if out, err := exec.Command(sox, wf.path, "-n", "stat").CombinedOutput(); err != nil {
				t.Errorf("sox could not decode it: %v\n%s", err, out)
			} else if wf.frames < 0 && !strings.Contains(string(out), "Samples read") {
				t.Errorf("sox stat printed no sample count:\n%s", out)
			}
		})
	}
}

// TestWriterAgreesWithFFprobe checks the same headers against ffprobe, and
// that ffmpeg decodes as many frames as were written.
func TestWriterAgreesWithFFprobe(t *testing.T) {
	if !*oracle {
		t.Skip("run with -oracle to cross-check against sox and ffprobe")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	for _, wf := range writeOracleFiles(t) {
		t.Run(filepath.Base(wf.path), func(t *testing.T) {
			out, err := exec.Command(ffprobe, "-v", "error", "-count_packets",
				"-show_entries", "stream=sample_rate,channels,duration_ts",
				"-of", "default=noprint_wrappers=1", wf.path).CombinedOutput()
			if err != nil {
				t.Fatalf("ffprobe: %v\n%s", err, out)
			}
			want := map[string]int64{"sample_rate": wf.rate, "channels": int64(wf.channels)}
			if wf.frames >= 0 {
				want["duration_ts"] = wf.frames
			}
			for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
				if !ok {
					continue
				}
				got, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					continue
				}
				if w, ok := want[key]; ok && w != got {
					t.Errorf("%s: ffprobe says %d, want %d", key, got, w)
				}
				delete(want, key)
			}
			for key := range want {
				t.Errorf("ffprobe did not report %s:\n%s", key, out)
			}
		})
	}
}

// TestG711DecodesAsSoxAndFFmpeg writes every code of each law and checks that
// sox and ffmpeg expand them to the levels this package does. Only decoding is
// compared: the encoders of the three differ wherever a sample lies near the
// edge of a step.
func TestG711DecodesAsSoxAndFFmpeg(t *testing.T) {
	if !*oracle {
		t.Skip("run with -oracle to cross-check against sox and ffprobe")
	}
	dir := t.TempDir()
	for _, f := range []pcm.Format{pcm.ALaw, pcm.MuLaw} {
		codes := make([]byte, 256)
		for i := range codes {
			codes[i] = byte(i)
		}
		x := make([]float64, len(codes))
		pcm.Decode(x, codes, f)
		l := Layout{Format: f, Rate: 8000, Channels: 1, Container: WAV}
		path := filepath.Join(dir, f.String()+".wav")
		file := writeAll(t, l, true, func(w *Writer) (int, error) { return w.WriteSamples(x) })
		if err := os.WriteFile(path, file, 0o600); err != nil {
			t.Fatal(err)
		}
		want := make([]byte, 2*len(x))
		pcm.Encode(want, x, pcm.I16LE)
		// Mu-law's negative zero is written as the positive one, which is the
		// same level.

		for _, tool := range []struct {
			name string
			args []string
		}{
			{"sox", []string{"-D", path, "-t", "raw", "-e", "signed-integer", "-b", "16", "-"}},
			{"ffmpeg", []string{"-v", "error", "-i", path, "-f", "s16le", "-"}},
		} {
			t.Run(f.String()+"/"+tool.name, func(t *testing.T) {
				bin, err := exec.LookPath(tool.name)
				if err != nil {
					t.Skip(tool.name + " is not installed")
				}
				got, err := exec.Command(bin, tool.args...).Output()
				if err != nil {
					t.Fatalf("%s: %v", tool.name, err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s decodes the codes to different levels", tool.name)
				}
			})
		}
	}
}
