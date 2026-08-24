package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/samuel/go-dsp/sampleio"
)

// The subcommands are called directly rather than through a built binary, which
// is why every flag set is ContinueOnError: an ExitOnError set would take the
// test binary down with it on the bad-flag cases below.

// writeRaw builds a headerless 16-bit capture holding an FSK signal, which is
// enough for every view including the tone track.
func writeRaw(t *testing.T, name string, seconds float64) string {
	t.Helper()
	const rate = 11025.0
	n := int(rate * seconds)
	var buf bytes.Buffer
	for i := range n {
		f := 1600.0
		if (i/37)%2 == 1 {
			f = 1800
		}
		v := 0.5 * math.Sin(2*math.Pi*f*float64(i)/rate)
		c := int16(math.Round(v * 32767))
		buf.WriteByte(byte(c))
		buf.WriteByte(byte(c >> 8))
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func parsesAsXML(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	for {
		_, err := d.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("%s is not well-formed XML: %v", path, err)
		}
	}
}

func TestSubcommandsWriteWhatTheySay(t *testing.T) {
	raw := writeRaw(t, "fsk.raw", 1)
	dir := t.TempDir()
	base := []string{"-rate", "11025", "-format", "i16le"}

	for _, tt := range []struct {
		cmd  string
		v    views
		args []string
	}{
		{"spectrum", views{spectrum: true}, nil},
		{"waveform", views{waveform: true}, nil},
		{"level", views{level: true}, nil},
		{"histogram", views{histogram: true}, nil},
		{"tones", views{tones: true}, []string{"-mark", "1600", "-space", "1800", "-baud", "300", "-quiet"}},
	} {
		t.Run(tt.cmd, func(t *testing.T) {
			out := filepath.Join(dir, tt.cmd+".svg")
			args := append(append([]string{}, base...), tt.args...)
			args = append(args, "-o", out, raw)
			if err := draw(tt.cmd, args, tt.v); err != nil {
				t.Fatal(err)
			}
			parsesAsXML(t, out)
		})
	}

	t.Run("spectrogram png", func(t *testing.T) {
		out := filepath.Join(dir, "sg.png")
		args := append(append([]string{}, base...), "-o", out, raw)
		if err := draw("spectrogram", args, views{spectrogram: true}); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close() //nolint:errcheck // a read-only file in a temp dir
		img, err := png.Decode(f)
		if err != nil {
			t.Fatalf("the raster does not decode: %v", err)
		}
		if img.Bounds().Dx() == 0 {
			t.Error("the raster is empty")
		}
	})
}

// TestAnalyzeWritesTheWholeSet is the single-pass fan-out: one read of the file
// produces every view.
func TestAnalyzeWritesTheWholeSet(t *testing.T) {
	raw := writeRaw(t, "fsk.raw", 1)
	dir := filepath.Join(t.TempDir(), "look")
	err := draw("analyze", []string{
		"-rate", "11025", "-format", "i16le", "-out", dir, "-quiet", raw,
	}, views{spectrogram: true, spectrum: true, waveform: true, level: true, histogram: true, stats: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"spectrogram.svg", "spectrogram.png", "spectrum.svg",
		"waveform.svg", "level.svg", "histogram.svg", "stats.txt",
	} {
		path := filepath.Join(dir, name)
		st, err := os.Stat(path)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if st.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
		if strings.HasSuffix(name, ".svg") {
			parsesAsXML(t, path)
		}
	}
}

func TestStatsJSONFromTheCommandLine(t *testing.T) {
	raw := writeRaw(t, "fsk.raw", 0.5)
	out := filepath.Join(t.TempDir(), "stats.json")
	err := draw("stats", []string{
		"-rate", "11025", "-format", "i16le", "-json", "-o", out, raw,
	}, views{stats: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// One document structure whatever the file holds: a consumer reads channels[0]
	// for a mono file and channels[1] for the right of a stereo one.
	var doc struct {
		Channels []map[string]any `json:"channels"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if len(doc.Channels) != 1 {
		t.Fatalf("got %d channels, want 1", len(doc.Channels))
	}
	if doc.Channels[0]["rate"] != 11025.0 {
		t.Errorf("rate %v, want 11025", doc.Channels[0]["rate"])
	}
	if _, ok := doc.Channels[0]["spectral"]; !ok {
		t.Error("the spectral figures are missing")
	}
}

// TestErrorPaths is the reason the flag sets are ContinueOnError: every one of
// these has to come back as an error rather than as an exit.
func TestErrorPaths(t *testing.T) {
	raw := writeRaw(t, "fsk.raw", 0.2)
	out := t.TempDir()

	for _, tt := range []struct {
		name string
		cmd  string
		v    views
		args []string
		want string
	}{
		{"unknown flag", "stats", views{stats: true},
			[]string{"-nonesuch", raw}, "flag provided but not defined"},
		{"raw with no rate", "stats", views{stats: true},
			[]string{"-format", "i16le", raw}, "sample rate"},
		{"raw with no format", "stats", views{stats: true},
			[]string{"-rate", "11025", raw}, "sample format"},
		{"unknown format", "stats", views{stats: true},
			[]string{"-format", "nonesuch", "-rate", "8000", raw}, "unknown sample format"},
		{"unknown window", "spectrum", views{spectrum: true},
			[]string{"-format", "i16le", "-rate", "11025", "-window", "nonesuch", raw}, "unknown window"},
		{"unknown palette", "spectrogram", views{spectrogram: true},
			[]string{"-format", "i16le", "-rate", "11025", "-palette", "viridsi",
				"-o", filepath.Join(out, "a.png"), raw}, "unknown palette"},
		{"png line chart", "spectrum", views{spectrum: true},
			[]string{"-format", "i16le", "-rate", "11025", "-o", filepath.Join(out, "b.png"), raw},
			"cannot write"},
		{"-o and -out together", "spectrum", views{spectrum: true},
			[]string{"-format", "i16le", "-rate", "11025", "-o", "x.svg", "-out", out, raw},
			"one or the other"},
		{"two files", "stats", views{stats: true},
			[]string{"-format", "i16le", "-rate", "11025", raw, raw}, "one file at a time"},
		{"tones with no tones", "tones", views{tones: true},
			[]string{"-format", "i16le", "-rate", "11025", raw}, "-mark and -space"},
		{"missing file", "stats", views{stats: true},
			[]string{"-format", "i16le", "-rate", "11025", filepath.Join(out, "nope.raw")}, "no such file"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := draw(tt.cmd, tt.args, tt.v)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

// TestWAVNeedsNoFlags is the README example: a WAV carries its rate and its
// encoding, so the bare command has to work.
func TestWAVNeedsNoFlags(t *testing.T) {
	out := filepath.Join(t.TempDir(), "stats.txt")
	err := draw("stats", []string{"-fft", "32", "-o", out,
		filepath.Join("..", "..", "sampleio", "testdata", "tone-i16le.wav")}, views{stats: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"i16le", "rate          8000 Hz", "peak"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the report does not mention %q:\n%s", want, b)
		}
	}
}

func TestWindowNamesAllResolve(t *testing.T) {
	for _, name := range windowNames() {
		if _, err := windowFunc(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := windowFunc("nonesuch"); err == nil {
		t.Error("an unknown window should be an error")
	}
}

// writeStereoRaw builds a two-channel capture whose channels differ in both
// level and content, so a per-channel report has something to tell apart.
func writeStereoRaw(t *testing.T, name string, seconds float64, invert bool) string {
	t.Helper()
	const rate = 8000.0
	n := int(rate * seconds)
	var buf bytes.Buffer
	for i := range n {
		l := 0.5 * math.Sin(2*math.Pi*440*float64(i)/rate)
		r := 0.25 * math.Sin(2*math.Pi*660*float64(i)/rate)
		if invert {
			r = -l
		}
		for _, v := range []float64{l, r} {
			c := int16(math.Round(v * 32767))
			buf.WriteByte(byte(c))
			buf.WriteByte(byte(c >> 8))
		}
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func stereoArgs(path string, extra ...string) []string {
	return append(append([]string{"-rate", "8000", "-format", "i16le", "-channels", "2"}, extra...), path)
}

// TestStereoStatsCoversEveryChannel is the point of the feature: analyzing the
// left half of a stereo file and saying nothing about the right is how a fault
// in the right goes unnoticed.
func TestStereoStatsCoversEveryChannel(t *testing.T) {
	raw := writeStereoRaw(t, "stereo.raw", 0.5, false)
	out := filepath.Join(t.TempDir(), "stats.txt")
	if err := draw("stats", stereoArgs(raw, "-o", out), views{stats: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"channel 0", "channel 1", "channels", "0 against 1", "correlation"} {
		if !strings.Contains(got, want) {
			t.Errorf("the report does not mention %q:\n%s", want, got)
		}
	}
	// The right channel is 6 dB down and a different tone, so the two sections
	// must not be identical.
	if a, b, ok := strings.Cut(got, "channel 1"); ok && strings.Contains(a, "channel 0") {
		if strings.Contains(a, "peak          0.250") == strings.Contains(b, "peak          0.250") {
			t.Errorf("both channels report the same peak, so only one was measured:\n%s", got)
		}
	}
}

// TestStereoReportsAnInvertedPair is the fault a per-channel report cannot see:
// each channel on its own is unremarkable, and folded to mono they are silence.
func TestStereoReportsAnInvertedPair(t *testing.T) {
	raw := writeStereoRaw(t, "inverted.raw", 0.5, true)
	out := filepath.Join(t.TempDir(), "stats.txt")
	if err := draw("stats", stereoArgs(raw, "-o", out), views{stats: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "inverted") || !strings.Contains(got, "cancel to silence") {
		t.Errorf("an inverted pair went unreported:\n%s", got)
	}
	if !strings.Contains(got, "correlation -1.0000") {
		t.Errorf("the correlation does not read -1:\n%s", got)
	}
}

// TestStereoWritesAChartPerChannel checks the naming, including that a run
// covering one channel keeps the plain name it always had.
func TestStereoWritesAChartPerChannel(t *testing.T) {
	raw := writeStereoRaw(t, "stereo.raw", 0.5, false)

	t.Run("a directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "look")
		err := draw("analyze", stereoArgs(raw, "-out", dir, "-quiet"),
			views{spectrogram: true, spectrum: true, waveform: true, level: true, histogram: true, stats: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{
			"spectrum-ch0.svg", "spectrum-ch1.svg",
			"waveform-ch0.svg", "waveform-ch1.svg",
			"level-ch0.svg", "level-ch1.svg",
			"histogram-ch0.svg", "histogram-ch1.svg",
			"spectrogram-ch0.png", "spectrogram-ch1.svg",
			"stats.txt", // one report, however many channels
		} {
			path := filepath.Join(dir, name)
			st, err := os.Stat(path)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			if st.Size() == 0 {
				t.Errorf("%s is empty", name)
			}
			if strings.HasSuffix(name, ".svg") {
				parsesAsXML(t, path)
			}
		}
		// One statistics document, not one per channel.
		if _, err := os.Stat(filepath.Join(dir, "stats-ch0.txt")); err == nil {
			t.Error("the statistics were split per channel; they are one report")
		}
	})

	t.Run("one file", func(t *testing.T) {
		dir := t.TempDir()
		out := filepath.Join(dir, "spec.svg")
		if err := draw("spectrum", stereoArgs(raw, "-o", out), views{spectrum: true}); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"spec-ch0.svg", "spec-ch1.svg"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
		if _, err := os.Stat(out); err == nil {
			t.Error("an unlabelled spec.svg was written as well")
		}
	})

	t.Run("narrowed to one channel", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "spec.svg")
		if err := draw("spectrum", stereoArgs(raw, "-channel", "1", "-o", out), views{spectrum: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(out); err != nil {
			t.Errorf("-channel 1 should keep the plain name: %v", err)
		}
	})

	t.Run("mixed down", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "spec.svg")
		if err := draw("spectrum", stereoArgs(raw, "-channel", "mix", "-o", out), views{spectrum: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(out); err != nil {
			t.Errorf("-channel mix should keep the plain name: %v", err)
		}
	})
}

// TestStereoStdoutNeedsOneChannel checks that the impossible case is an error
// naming the ways out of it, rather than a silent choice of the first channel.
func TestStereoStdoutNeedsOneChannel(t *testing.T) {
	raw := writeStereoRaw(t, "stereo.raw", 0.2, false)
	err := draw("spectrum", stereoArgs(raw), views{spectrum: true})
	if err == nil {
		t.Fatal("want an error")
	}
	for _, want := range []string{"-channel", "-out"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

// TestStereoJSONHasBothChannels pins the document structure, which is the same
// whether the file has one channel or eight.
func TestStereoJSONHasBothChannels(t *testing.T) {
	raw := writeStereoRaw(t, "stereo.raw", 0.5, true)
	out := filepath.Join(t.TempDir(), "stats.json")
	if err := draw("stats", stereoArgs(raw, "-json", "-o", out), views{stats: true}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Channels []map[string]any `json:"channels"`
		Pairs    []map[string]any `json:"pairs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if len(doc.Channels) != 2 {
		t.Fatalf("got %d channels, want 2", len(doc.Channels))
	}
	if len(doc.Pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(doc.Pairs))
	}
	if c := doc.Pairs[0]["correlation"].(float64); math.Abs(c+1) > 1e-3 {
		t.Errorf("correlation %v, want -1 for an inverted pair", c)
	}
	if inv, _ := doc.Pairs[0]["inverted"].(bool); !inv {
		t.Error("the pair is not flagged as inverted")
	}
}

// TestChannelPairsScale checks the rule that keeps a wide file's comparison
// table from going quadratic.
func TestChannelPairsScale(t *testing.T) {
	for _, tc := range []struct{ n, want int }{
		{1, 0}, {2, 1}, {3, 3}, {8, 28}, {9, 8}, {16, 15},
	} {
		if got := len(channelPairs(tc.n)); got != tc.want {
			t.Errorf("%d channels give %d pairs, want %d", tc.n, got, tc.want)
		}
	}
	// Past the cutoff every comparison is against the first channel, so nothing
	// is compared with itself and none is repeated.
	for _, p := range channelPairs(12) {
		if p[0] != 0 || p[1] == 0 {
			t.Errorf("pair %v is not against channel 0", p)
		}
	}
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
// The notes go to standard error, so nothing else can see them.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	os.Stderr = saved
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestHelpExitsZero checks that -h is a request that was granted. The flag
// package prints the usage itself and hands back flag.ErrHelp, which main used
// to treat as a failure -- so asking a subcommand how to use it exited 1 and
// printed the error alongside the usage it had just been given.
func TestHelpExitsZero(t *testing.T) {
	for _, cmd := range []struct {
		name string
		v    views
	}{
		{"spectrum", views{spectrum: true}},
		{"spectrogram", views{spectrogram: true}},
		{"tones", views{tones: true}},
		{"stats", views{stats: true}},
		{"analyze", views{spectrogram: true, spectrum: true, stats: true}},
	} {
		t.Run(cmd.name, func(t *testing.T) {
			for _, flag := range []string{"-h", "-help"} {
				var err error
				usage := captureStderr(t, func() { err = draw(cmd.name, []string{flag}, cmd.v) })
				if err != nil {
					t.Errorf("%s: %v", flag, err)
				}
				// The usage really was printed, so this is not passing by
				// returning nil and doing nothing.
				if !strings.Contains(usage, "-rate") {
					t.Errorf("%s printed no usage: %q", flag, usage)
				}
			}
		})
	}
}

// TestStartPastTheEnd covers both routes to the same complaint. A file whose
// length has been checked against the size on disk knows at once; a stream does
// not, so the skip running out is what reports it.
func TestStartPastTheEnd(t *testing.T) {
	raw := writeRaw(t, "fsk.raw", 0.2) // 0.2 s at 11025 Hz
	out := filepath.Join(t.TempDir(), "stats.txt")

	t.Run("a file", func(t *testing.T) {
		err := draw("stats", []string{"-format", "i16le", "-rate", "11025",
			"-start", "100", "-o", out, raw}, views{stats: true})
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "-start 100") || !strings.Contains(err.Error(), "past the end") {
			t.Errorf("the error does not name -start and the end of the file: %v", err)
		}
	})

	t.Run("a stream", func(t *testing.T) {
		// Standard input has no size to check the header against, so Frames is
		// -1 and open cannot tell. The read has to notice instead.
		f, err := os.Open(raw)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close() //nolint:errcheck // a test fixture
		saved := os.Stdin
		os.Stdin = f
		defer func() { os.Stdin = saved }()

		err = draw("stats", []string{"-format", "i16le", "-rate", "11025",
			"-start", "100", "-o", out, "-"}, views{stats: true})
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "-start 100") {
			t.Errorf("the error does not name -start: %v", err)
		}
	})

	// A start inside the file is not an error, so the check is not simply
	// rejecting every -start.
	if err := draw("stats", []string{"-format", "i16le", "-rate", "11025",
		"-start", "0.05", "-o", out, raw}, views{stats: true}); err != nil {
		t.Errorf("a start inside the file: %v", err)
	}
}

// TestHopNotePrintsOnce is the reason stftOptions is called once in run rather
// than once per channel: the notes it produces describe the analysis, not the
// channel, so a stereo file used to be told twice that its hop had been
// clamped.
func TestHopNotePrintsOnce(t *testing.T) {
	raw := writeStereoRaw(t, "stereo.raw", 0.5, false)
	dir := t.TempDir()

	var err error
	// A hop wider than half the frame, which is what produces the note.
	notes := captureStderr(t, func() {
		err = draw("spectrogram", stereoArgs(raw, "-fft", "256", "-hop", "4000", "-out", dir),
			views{spectrogram: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = "would skip past whole frames"
	if n := strings.Count(notes, want); n != 1 {
		t.Errorf("the hop note printed %d times, want once:\n%s", n, notes)
	}
}

// TestStdoutChannelCountIsCheckedBeforeReading pins the ordering, which the
// message alone cannot show: a run that cannot put its output anywhere should
// say so in the first moment rather than after minutes of analysis.
//
// /dev/zero is a source with no end, so a check that came after the read would
// never return at all.
func TestStdoutChannelCountIsCheckedBeforeReading(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero on this platform")
	}
	done := make(chan error, 1)
	go func() {
		done <- draw("spectrum", stereoArgs("/dev/zero"), views{spectrum: true})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "-channel") {
			t.Errorf("the error does not name the way out: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the channel count was not checked before the read: an endless source never returned")
	}
}

// writeIQRaw builds a headerless interleaved u8 I/Q capture holding a complex
// exponential, which is what rtl-sdr writes and what the two-sided views exist
// for. The tone is at a positive offset, so a run that mixed up I and Q would
// put it on the wrong side of zero.
func writeIQRaw(t *testing.T, name string, seconds float64) string {
	t.Helper()
	const rate = 8000.0
	n := int(rate * seconds)
	buf := make([]byte, 0, 2*n)
	for i := range n {
		a := 2 * math.Pi * 1000 * float64(i) / rate
		buf = append(buf,
			byte(math.Round(0.5*math.Cos(a)*127)+128),
			byte(math.Round(0.5*math.Sin(a)*127)+128))
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestIQRunCoversTheComplexPath drives the whole complex half of the command
// from the outside. Every sink here has a WriteComplex alongside its Write and
// the reading loop has a second form of its own, and none of it was reached by
// a real signal: the format spelling, the two-sided spectrogram, the imbalance
// figures and the mirror check are one path from -format cu8 through to the
// document.
func TestIQRunCoversTheComplexPath(t *testing.T) {
	raw := writeIQRaw(t, "iq.cu8", 0.5)
	dir := t.TempDir()

	err := draw("analyze", []string{"-format", "cu8", "-rate", "8000", "-fft", "256",
		"-out", dir, "-quiet", raw},
		views{spectrogram: true, spectrum: true, waveform: true, level: true, histogram: true, stats: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"spectrogram.png", "spectrum.svg", "waveform.svg",
		"level.svg", "histogram.svg", "stats.txt",
	} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if st.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}

	b, err := os.ReadFile(filepath.Join(dir, "stats.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// The figures only a complex analysis produces. A run that fell back to
	// reading the file as two real channels would report neither.
	for _, want := range []string{"cu8", "iq imbalance", "quadrature error"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the report does not mention %q:\n%s", want, b)
		}
	}

	// The JSON carries the same section, which is the other reader of it.
	out := filepath.Join(t.TempDir(), "stats.json")
	if err := draw("stats", []string{"-format", "cu8", "-rate", "8000", "-fft", "256",
		"-json", "-o", out, raw}, views{stats: true}); err != nil {
		t.Fatal(err)
	}
	jb, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Channels []struct {
			Complex bool `json:"complex"`
			IQ      *struct {
				GainImbalanceDB float64 `json:"gainImbalanceDB"`
			} `json:"iq"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(jb, &doc); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}
	if len(doc.Channels) != 1 {
		t.Fatalf("channels = %d, want one for an I/Q pair", len(doc.Channels))
	}
	if !doc.Channels[0].Complex {
		t.Error("the channel is not marked complex")
	}
	if doc.Channels[0].IQ == nil {
		t.Fatal("no iq section")
	}
	// A synthetic exponential is perfectly balanced apart from the 8-bit
	// quantization, so the figure has to come back near zero rather than
	// unset or absurd.
	if g := doc.Channels[0].IQ.GainImbalanceDB; math.Abs(g) > 0.5 {
		t.Errorf("gain imbalance %v dB, want near 0 for a synthetic exponential", g)
	}

	// A real analysis of an I/Q file, and an I/Q analysis of a real file, are
	// both mistakes the flags can express and the run has to refuse.
	if err := draw("tones", []string{"-format", "cu8", "-rate", "8000",
		"-mark", "500", "-space", "1500", "-o", filepath.Join(t.TempDir(), "t.svg"), raw},
		views{tones: true}); err == nil {
		t.Error("the tone track accepted an I/Q stream")
	}
}

// TestFormatsListsEveryFormat covers the one subcommand that takes no file. It
// prints to standard output, and every name it prints has to be one -format
// accepts, or the help is telling the user to pass something that will be
// rejected.
func TestFormatsListsEveryFormat(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	formats()
	os.Stdout = saved
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	for _, f := range sampleio.Formats() {
		if !strings.Contains(out, f.String()) {
			t.Errorf("the listing omits %q:\n%s", f, out)
		}
		if _, _, err := sampleio.ParseFormatSpec(f.String()); err != nil {
			t.Errorf("%q is listed but -format rejects it: %v", f, err)
		}
		// And the c-prefixed spelling the note describes.
		if _, iq, err := sampleio.ParseFormatSpec("c" + f.String()); err != nil || !iq {
			t.Errorf("c%s: iq=%v, err=%v", f, iq, err)
		}
	}
	if !strings.Contains(out, "cu8") {
		t.Errorf("the listing does not explain the c prefix:\n%s", out)
	}
}
