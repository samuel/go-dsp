//go:build ignore

// Generate the WAV fixtures with sox, then run:
//
//	go generate ./sampleio
//
// Only the containers are committed. The raw formats are built byte by byte in
// the tests, where the expected float64 can be computed from the integer codes
// and nothing has to be taken on trust.
//
// The headers deliberately come from sox rather than from a writer in this
// package. A parser checked against its own writer passes when both halves are
// wrong in the same way, and the reason for these files is that something
// else wrote them: sox emits WAVE_FORMAT_EXTENSIBLE for 24- and 32-bit, a
// format chunk of 18 bytes for float, a fact chunk that has to be skipped, and
// the pad byte after an odd chunk. None of that would appear in fixtures
// written here.
package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
)

// The signal every fixture holds: a tone at bin 3 of 64, so it is coherent with
// a 64-point transform and a test can look for it in exactly one bin.
const (
	frames = 64
	bin    = 3
	rate   = 8000
	amp    = 0.5
)

func tone(n int) []float64 {
	x := make([]float64, n)
	for i := range x {
		// The phase argument is reduced in integers, the way dspviz.Tone does
		// it, so it stays exact.
		x[i] = amp * math.Cos(2*math.Pi*float64((bin*i)%n)/float64(n))
	}
	return x
}

func main() {
	// Run either from the package directory, via go generate, or from inside
	// testdata by hand.
	dir := "testdata"
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = "."
	}

	// A float64 source, so every fixture is converted *down* to its own width
	// and actually carries the precision it claims. Starting from 16-bit would
	// leave the 24-bit, 32-bit and float files holding a 16-bit signal, and a
	// test could not tell a working wide decoder from a broken one.
	src := filepath.Join(dir, "src.tmp.raw")
	x := tone(frames)
	pcm := make([]byte, 8*len(x))
	for i, v := range x {
		binary.LittleEndian.PutUint64(pcm[i*8:], math.Float64bits(v))
	}
	if err := os.WriteFile(src, pcm, 0o644); err != nil {
		log.Fatal(err)
	}
	defer os.Remove(src)

	// How to read that source back.
	from := []string{"-t", "raw", "-r", fmt.Sprint(rate), "-e", "floating-point", "-b", "64", "-c", "1", src}

	for _, c := range []struct {
		name     string
		enc      string
		bits     int
		channels int
		extra    []string
	}{
		{name: "tone-i16le.wav", enc: "signed-integer", bits: 16, channels: 1},
		{name: "tone-i24le.wav", enc: "signed-integer", bits: 24, channels: 1},
		{name: "tone-i32le.wav", enc: "signed-integer", bits: 32, channels: 1},
		{name: "tone-u8.wav", enc: "unsigned-integer", bits: 8, channels: 1},
		{name: "tone-f32le.wav", enc: "floating-point", bits: 32, channels: 1},
		{name: "tone-f64le.wav", enc: "floating-point", bits: 64, channels: 1},
		// Two channels, the second inverted, so a test can tell which one it
		// selected and prove a mixdown of the two is silence.
		{name: "stereo-i16le.wav", enc: "signed-integer", bits: 16, channels: 2},
	} {
		out := filepath.Join(dir, c.name)
		args := append([]string(nil), from...)
		if c.channels == 2 {
			// Build the pair by hand so the second channel is the negation of
			// the first; sox would just duplicate it. That makes a mixdown
			// exactly silence, which is a stronger check than a plausible mean.
			st := filepath.Join(dir, "stereo.tmp.raw")
			pair := make([]byte, 16*len(x))
			for i, v := range x {
				binary.LittleEndian.PutUint64(pair[i*16:], math.Float64bits(v))
				binary.LittleEndian.PutUint64(pair[i*16+8:], math.Float64bits(-v))
			}
			if err := os.WriteFile(st, pair, 0o644); err != nil {
				log.Fatal(err)
			}
			defer os.Remove(st)
			args = []string{"-t", "raw", "-r", fmt.Sprint(rate), "-e", "floating-point", "-b", "64", "-c", "2", st}
		}
		args = append(args, "-e", c.enc, "-b", fmt.Sprint(c.bits), out)
		args = append(args, c.extra...)

		cmd := exec.Command("sox", args...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("sox %v: %v", args, err)
		}
		fmt.Println("wrote", c.name)
	}

	// RF64, which only ffmpeg writes: a 64-bit ds64 chunk standing in for the
	// 32-bit RIFF and data sizes.
	rf := filepath.Join(dir, "tone-rf64.wav")
	cmd := exec.Command("ffmpeg", "-v", "error", "-y",
		"-f", "f64le", "-ar", fmt.Sprint(rate), "-ac", "1", "-i", src,
		"-c:a", "pcm_s16le", "-rf64", "always", rf)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("ffmpeg: %v", err)
	}
	fmt.Println("wrote tone-rf64.wav")
}
