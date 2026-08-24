package sampleio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The signal every fixture in testdata holds. Kept in step with
// testdata/generate.go: a tone at bin 3 of 64 frames, half scale.
const (
	fixFrames = 64
	fixBin    = 3
	fixRate   = 8000
	fixAmp    = 0.5
)

func fixtureTone() []float64 {
	x := make([]float64, fixFrames)
	for i := range x {
		x[i] = fixAmp * math.Cos(2*math.Pi*float64((fixBin*i)%fixFrames)/fixFrames)
	}
	return x
}

// TestOpenWAVFixtures reads every container sox and ffmpeg wrote. The headers
// are third-party on purpose: sox emits WAVE_FORMAT_EXTENSIBLE for 24- and
// 32-bit, an 18-byte format chunk for float, and a fact chunk that has to be
// skipped, none of which a writer in this package would produce.
func TestOpenWAVFixtures(t *testing.T) {
	for _, tc := range []struct {
		file      string
		want      Format
		bits      int
		codec     string
		channels  int
		tolerance float64
	}{
		{"tone-u8.wav", U8, 8, "pcm", 1, 1.0 / 128},
		{"tone-i16le.wav", I16LE, 16, "pcm", 1, 1.0 / 32768},
		{"tone-i24le.wav", I24LE, 24, "pcm", 1, 1.0 / 8388608},
		{"tone-i32le.wav", I32LE, 32, "pcm", 1, 1e-9},
		{"tone-f32le.wav", F32LE, 32, "float", 1, 1e-7},
		// sox works in 32-bit internally, so even its float64 output carries
		// only about 2^-31 of precision; the fixture tests this decoder, not
		// sox's arithmetic.
		{"tone-f64le.wav", F64LE, 64, "float", 1, 1e-9},
		{"tone-rf64.wav", I16LE, 16, "pcm", 1, 1.0 / 32768},
		{"stereo-i16le.wav", I16LE, 16, "pcm", 2, 1.0 / 32768},
	} {
		t.Run(tc.file, func(t *testing.T) {
			r, err := Open(filepath.Join("testdata", tc.file), Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on

			info := r.Info()
			if info.Container != "wav" {
				t.Errorf("Container = %q, want wav", info.Container)
			}
			if info.Format != tc.want {
				t.Errorf("Format = %v, want %v", info.Format, tc.want)
			}
			if info.Rate != fixRate {
				t.Errorf("Rate = %v, want %v", info.Rate, fixRate)
			}
			if info.Channels != tc.channels {
				t.Errorf("Channels = %d, want %d", info.Channels, tc.channels)
			}
			if info.Bits != tc.bits {
				t.Errorf("Bits = %d, want %d", info.Bits, tc.bits)
			}
			if info.Codec != tc.codec {
				t.Errorf("Codec = %q, want %q", info.Codec, tc.codec)
			}
			if info.Frames != fixFrames {
				t.Errorf("Frames = %d, want %d", info.Frames, fixFrames)
			}

			// The samples have to be the tone that went in, to within what the
			// container's own width can hold.
			got := readAll(t, r, 13)
			want := fixtureTone()
			if len(got) != len(want) {
				t.Fatalf("read %d frames, want %d", len(got), len(want))
			}
			// sox dithers when it narrows, so allow a couple of steps rather
			// than one.
			tol := 3 * tc.tolerance
			for i := range want {
				if math.Abs(got[i]-want[i]) > tol {
					t.Errorf("frame %d = %v, want %v within %v", i, got[i], want[i], tol)
					break
				}
			}
		})
	}
}

// TestOpenWAVChannelSelection uses the stereo fixture, whose right channel is
// the negation of its left to within the rounding sox did on the way to 16 bits,
// so a mixdown is silence to within half a code.
func TestOpenWAVChannelSelection(t *testing.T) {
	path := filepath.Join("testdata", "stereo-i16le.wav")
	want := fixtureTone()

	for _, tc := range []struct {
		name string
		pick Channel
		sign float64
		zero bool
	}{
		{"left", 0, 1, false},
		{"right", 1, -1, false},
		{"mix", MixChannels, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Open(path, Options{Channel: tc.pick})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
			got := readAll(t, r, 16)
			if len(got) != fixFrames {
				t.Fatalf("read %d frames, want %d", len(got), fixFrames)
			}
			for i := range got {
				expect := tc.sign * want[i]
				if tc.zero {
					expect = 0
				}
				// The tolerance is a code or two rather than a fudge factor:
				// the generator negates in float64 and sox rounds the pair
				// independently on the way to 16 bits, so the two channels
				// disagree by at most one code and the mixdown by half of one.
				// TestOpenWAVPlanar measures that directly.
				if math.Abs(got[i]-expect) > 3.0/32768 {
					t.Errorf("frame %d = %v, want %v", i, got[i], expect)
					break
				}
			}
		})
	}
}

// TestOpenWAVPlanar reads the stereo fixture through the planar path and checks
// it against a Reader per channel, exactly. The two decode the same bytes with
// the same arithmetic, so any difference is a bug rather than rounding.
//
// It also measures how far the fixture's channels are from exact negations:
// sox rounds each channel independently on the way to 16 bits, so half the
// frames disagree by one code. The tolerance in TestOpenWAVChannelSelection is
// there because of it.
func TestOpenWAVPlanar(t *testing.T) {
	path := filepath.Join("testdata", "stereo-i16le.wav")

	want := make([][]float64, 2)
	for c := range want {
		r, err := Open(path, Options{Channel: Channel(c)})
		if err != nil {
			t.Fatal(err)
		}
		want[c] = readAll(t, r, 16)
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}

	r, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	got := readAllPlanar(t, r, 2, 16)

	for c := range got {
		if len(got[c]) != fixFrames {
			t.Fatalf("channel %d: read %d frames, want %d", c, len(got[c]), fixFrames)
		}
		for i := range got[c] {
			if got[c][i] != want[c][i] {
				t.Fatalf("channel %d frame %d = %v, want %v", c, i, got[c][i], want[c][i])
			}
		}
	}

	// One code of asymmetry, not zero and not a fudge factor.
	const code = 1.0 / 32768
	var worst float64
	for i := range got[0] {
		worst = math.Max(worst, math.Abs(got[0][i]+got[1][i]))
	}
	if worst > code {
		t.Errorf("the channels differ from an exact negation by %v, more than the one code sox rounds by", worst)
	}
	if worst == 0 {
		t.Error("the channels now negate exactly; the tolerances that exist for the rounding can go")
	}
}

// TestOpenWAVRateOverride records rather than hides a disagreement.
func TestOpenWAVRateOverride(t *testing.T) {
	r, err := Open(filepath.Join("testdata", "tone-i16le.wav"), Options{Rate: 44100})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	info := r.Info()
	if info.Rate != 44100 {
		t.Errorf("Rate = %v, want the override 44100", info.Rate)
	}
	if len(info.Notes) == 0 || !strings.Contains(strings.Join(info.Notes, " "), "8000") {
		t.Errorf("Notes = %q, want one naming the header rate", info.Notes)
	}
}

// TestOpenWAVAsRaw is the escape hatch for a file whose header is wrong: the
// header bytes come through as samples, so there are more of them.
func TestOpenWAVAsRaw(t *testing.T) {
	r, err := Open(filepath.Join("testdata", "tone-i16le.wav"), Options{
		Raw: true, Format: I16LE, Rate: 8000,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	if got := r.Info().Container; got != "raw" {
		t.Errorf("Container = %q, want raw", got)
	}
	if got := r.Info().Frames; got <= fixFrames {
		t.Errorf("Frames = %d, want more than %d, since the header is data now", got, fixFrames)
	}
}

// wav builds a container around a payload, for the malformed table below. It is
// a test fixture and not a writer: the committed fixtures come from sox, so this
// is only ever used to make a header deliberately wrong.
func wav(fmtChunk []byte, dataSize uint32, payload []byte, extra ...[]byte) []byte {
	return wavTrailing(fmtChunk, dataSize, payload, extra, nil)
}

// wavTrailing also puts chunks after the data chunk, which is what a real writer
// does and what the data size therefore has to bound.
func wavTrailing(fmtChunk []byte, dataSize uint32, payload []byte, before, after [][]byte) []byte {
	var b bytes.Buffer
	body := new(bytes.Buffer)
	body.WriteString("fmt ")
	_ = binary.Write(body, binary.LittleEndian, uint32(len(fmtChunk)))
	body.Write(fmtChunk)
	if len(fmtChunk)%2 == 1 {
		body.WriteByte(0)
	}
	for _, e := range before {
		body.Write(e)
	}
	body.WriteString("data")
	_ = binary.Write(body, binary.LittleEndian, dataSize)
	body.Write(payload)
	for _, e := range after {
		body.Write(e)
	}

	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(4+body.Len()))
	b.WriteString("WAVE")
	b.Write(body.Bytes())
	return b.Bytes()
}

// pcmFmt is a well-formed 16-byte format chunk.
func pcmFmt(tag uint16, channels int, rate uint32, bits int) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:], tag)
	binary.LittleEndian.PutUint16(b[2:], uint16(channels))
	binary.LittleEndian.PutUint32(b[4:], rate)
	binary.LittleEndian.PutUint32(b[8:], rate*uint32(channels*bits/8))
	binary.LittleEndian.PutUint16(b[12:], uint16(channels*bits/8))
	binary.LittleEndian.PutUint16(b[14:], uint16(bits))
	return b
}

// TestWAVMalformed is the table of things a real file does wrong. Each case must
// produce an error or a note, never a panic and never silent garbage.
func TestWAVMalformed(t *testing.T) {
	good := pcmFmt(wavPCM, 1, 8000, 16)
	payload := i16(0.5, -0.5, 0.25, -0.25)

	for _, tc := range []struct {
		name    string
		in      []byte
		wantErr bool
		frames  int64 // frames that must come out, when wantErr is false
	}{
		{"well formed", wav(good, uint32(len(payload)), payload), false, 4},

		// A streaming writer cannot know the size, so these three mean "to the
		// end of the file" rather than "empty".
		{"placeholder data size", wav(good, 0xffffffff, payload), false, 4},
		{"zero data size", wav(good, 0, payload), false, 4},

		// A chunk of odd size is followed by a pad byte the size does not
		// count. Missing it desynchronizes everything after it.
		{"odd chunk skipped correctly", wav(good, uint32(len(payload)), payload,
			chunk("LIST", []byte{1, 2, 3})), false, 4},
		{"unknown chunks are skipped", wav(good, uint32(len(payload)), payload,
			chunk("fact", []byte{0x40, 0, 0, 0}), chunk("PEAK", make([]byte, 16))), false, 4},

		{"truncated riff header", []byte("RIFF"), true, 0},
		{"riff but not wave", append([]byte("RIFF\x00\x00\x00\x00"), []byte("AVI ")...), true, 0},
		{"no data chunk", func() []byte {
			b := wav(good, 4, payload)
			return b[:len(b)-len(payload)-8]
		}(), true, 0},
		{"twelve bits", wav(pcmFmt(wavPCM, 1, 8000, 12), uint32(len(payload)), payload), true, 0},
		{"zero channels", wav(pcmFmt(wavPCM, 0, 8000, 16), uint32(len(payload)), payload), true, 0},
		{"zero rate", wav(pcmFmt(wavPCM, 1, 0, 16), uint32(len(payload)), payload), true, 0},
		{"format chunk too short", wav(good[:12], uint32(len(payload)), payload), true, 0},
		{"adpcm", wav(pcmFmt(2, 1, 8000, 4), uint32(len(payload)), payload), true, 0},
		{"a-law", wav(pcmFmt(wavALaw, 1, 8000, 8), uint32(len(payload)), payload), true, 0},
		{"mu-law", wav(pcmFmt(wavMuLaw, 1, 8000, 8), uint32(len(payload)), payload), true, 0},
		{"unknown tag", wav(pcmFmt(0x1234, 1, 8000, 16), uint32(len(payload)), payload), true, 0},
		{"extensible but too short", wav(pcmFmt(wavExtensible, 1, 8000, 16), uint32(len(payload)), payload), true, 0},
		{"extensible with a foreign guid", wav(extFmt(1, 8000, 16, 16, foreignGUID()),
			uint32(len(payload)), payload), true, 0},

		// A block size that disagrees with the arithmetic is survivable: the
		// samples are usually laid out correctly anyway.
		{"inconsistent block align", wav(badAlign(good), uint32(len(payload)), payload), false, 4},

		// The format chunk has to arrive first: the walk stops at the data chunk and
		// nothing else can decode its samples. The reverse order is rejected
		// rather than read with the zero value of Format.
		{"data before fmt", dataBeforeFmt(good, payload), true, 0},

		// An extensible format chunk hides the real tag in the first two bytes
		// of a 16-byte GUID, so it needs 22 extra bytes; a smaller cbSize
		// describes a chunk that cannot hold one.
		{"extensible cbSize below 22", wav(shortCBSize(extFmt(1, 8000, 16, 16, ksGUID(wavPCM)), 21),
			uint32(len(payload)), payload), true, 0},
		{"extensible cbSize zero", wav(shortCBSize(extFmt(1, 8000, 16, 16, ksGUID(wavPCM)), 0),
			uint32(len(payload)), payload), true, 0},

		// A chunk header that promises more bytes than the file holds. Each of
		// these truncates inside a different chunk, so each takes a different
		// path out of the walk.
		{"EOF inside fmt", truncateBy(wav(good, uint32(len(payload)), payload), len(payload)+8+4), true, 0},
		{"EOF inside a skipped chunk", truncateBy(wav(good, uint32(len(payload)), payload,
			chunk("LIST", make([]byte, 64))), len(payload)+8+40), true, 0},
		{"EOF inside a chunk header", truncateBy(wav(good, uint32(len(payload)), payload), len(payload)+4), true, 0},
		{"EOF before the pad byte", truncateBy(wav(good, uint32(len(payload)), payload,
			chunk("LIST", []byte{1, 2, 3})), len(payload)+8+1), true, 0},

		// RF64 carries the real sizes in a ds64 chunk. Fewer than 16 bytes of
		// it does not reach the data size, so the 32-bit field stands and the
		// file still reads -- which is the point: a short ds64 is a header this
		// package cannot use, not a file it cannot read.
		{"ds64 shorter than 16 bytes", rf64(good, uint32(len(payload)), payload, 8), false, 4},
		{"ds64 empty", rf64(good, uint32(len(payload)), payload, 0), false, 4},
		{"ds64 whole", rf64(good, uint32(len(payload)), payload, 28), false, 4},
		{"ds64 truncated inside itself", truncateBy(rf64(good, uint32(len(payload)), payload, 28),
			len(payload)+8+20), true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(tc.in), Options{})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted a malformed file; Info = %+v", r.Info())
				}
				return
			}
			if err != nil {
				t.Fatalf("rejected a file it should read: %v", err)
			}
			// A stream's length cannot be checked against anything, so it is
			// reported as unknown whatever the header said.
			if got := r.Info().Frames; got != -1 {
				t.Errorf("Frames = %d, want -1 for a stream", got)
			}
			// Whatever the header said, the payload has to come out.
			if got := len(readAll(t, r, 3)); int64(got) != tc.frames {
				t.Errorf("read %d frames, want %d", got, tc.frames)
			}
		})
	}
}

func chunk(id string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString(id)
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(payload)))
	b.Write(payload)
	if len(payload)%2 == 1 {
		b.WriteByte(0) // the pad byte, which the size does not count
	}
	return b.Bytes()
}

// extFmt builds a 40-byte WAVE_FORMAT_EXTENSIBLE format chunk.
func extFmt(channels int, rate uint32, bits, validBits int, guid []byte) []byte {
	b := make([]byte, 40)
	copy(b, pcmFmt(wavExtensible, channels, rate, bits))
	binary.LittleEndian.PutUint16(b[16:], 22)
	binary.LittleEndian.PutUint16(b[18:], uint16(validBits))
	binary.LittleEndian.PutUint32(b[20:], 0)
	copy(b[24:], guid)
	return b
}

func foreignGUID() []byte {
	g := make([]byte, 16)
	binary.LittleEndian.PutUint16(g, wavPCM)
	for i := 2; i < 16; i++ {
		g[i] = 0xee // not the KSDATAFORMAT suffix
	}
	return g
}

func badAlign(fmtChunk []byte) []byte {
	b := append([]byte(nil), fmtChunk...)
	binary.LittleEndian.PutUint16(b[12:], 99)
	return b
}

// TestWAVTrailingChunks is a regression for a real file: GoldWave writes a
// LIST INFO chunk after the data chunk, and radio-paket300.wav in the repo root
// carries 82 bytes of it -- which this package once decoded as 82 extra samples,
// because the data chunk size bounded the reported length but not the read. The
// synthetic fixtures all end at their data, so only a real capture showed it.
func TestWAVTrailingChunks(t *testing.T) {
	payload := i16(0.5, -0.5, 0.25, -0.25)
	info := chunk("LIST", append([]byte("INFOISFT"), "File created by something else."...))

	raw := wavTrailing(pcmFmt(wavPCM, 1, 8000, 16), uint32(len(payload)), payload, nil, [][]byte{info})
	path := filepath.Join(t.TempDir(), "trailer.wav")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	if got := r.Info().Frames; got != 4 {
		t.Errorf("Frames = %d, want 4", got)
	}
	if got := readAll(t, r, 3); len(got) != 4 {
		t.Errorf("read %d frames, want 4; the trailer was decoded as samples", len(got))
	}

	// The bound applies to a pipe too, where the length is not reported.
	sr, err := NewReader(bytes.NewReader(raw), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(readAll(t, sr, 3)); got != 4 {
		t.Errorf("from a stream, read %d frames, want 4", got)
	}
}

// TestWAVStreamLengthIsNotAGuess is why a stream reports no length. sox writes a
// data size of 0x3ffff800 down a pipe -- 6.8 hours at 44.1 kHz -- so a caller
// that believed it would size a transform for a file thousands of times longer
// than the one it is reading.
func TestWAVStreamLengthIsNotAGuess(t *testing.T) {
	payload := i16(0.5, -0.5, 0.25, -0.25)
	raw := wav(pcmFmt(wavPCM, 1, 8000, 16), 0x3ffff800, payload)

	r, err := NewReader(bytes.NewReader(raw), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Info().Frames; got != -1 {
		t.Errorf("Frames = %d, want -1 rather than a header's unchecked claim", got)
	}
	if got := len(readAll(t, r, 3)); got != 4 {
		t.Errorf("read %d frames, want the 4 that are there", got)
	}

	// A caller's own Limit needs no checking, so it is reported.
	lim, err := NewReader(bytes.NewReader(raw), Options{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := lim.Info().Frames; got != 3 {
		t.Errorf("Frames = %d, want the caller's own limit of 3", got)
	}
}

// TestWAVFramesIsWhatComesOut is the contract Frames makes, checked across every
// fixture rather than asserted once.
func TestWAVFramesIsWhatComesOut(t *testing.T) {
	for _, name := range []string{
		"tone-u8.wav", "tone-i16le.wav", "tone-i24le.wav", "tone-i32le.wav",
		"tone-f32le.wav", "tone-f64le.wav", "tone-rf64.wav", "stereo-i16le.wav",
	} {
		r, err := Open(filepath.Join("testdata", name), Options{})
		if err != nil {
			t.Fatal(err)
		}
		want := r.Info().Frames
		got := int64(len(readAll(t, r, 7)))
		_ = r.Close()
		if got != want {
			t.Errorf("%s: Frames said %d, read %d", name, want, got)
		}
	}
}

// TestWAVDataSizePastTheEnd is the case only a file can catch: a writer that
// streamed to disk and never went back to fix the size. The file's own length
// is the answer, and the claim is recorded rather than believed.
func TestWAVDataSizePastTheEnd(t *testing.T) {
	payload := i16(0.5, -0.5, 0.25, -0.25)
	path := filepath.Join(t.TempDir(), "lying.wav")
	if err := os.WriteFile(path, wav(pcmFmt(wavPCM, 1, 8000, 16), 1<<20, payload), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck // a file opened for reading has no close error a test could act on
	if got := r.Info().Frames; got != -1 {
		t.Errorf("Frames = %d, want -1 for a size the file cannot back", got)
	}
	if notes := strings.Join(r.Info().Notes, " "); !strings.Contains(notes, "more than the file holds") {
		t.Errorf("Notes = %q, want one saying the size was not believed", r.Info().Notes)
	}
	if got := readAll(t, r, 3); len(got) != 4 {
		t.Errorf("read %d frames, want the 4 that are there", len(got))
	}
}

// TestWAVExtensibleFromSox checks the extensible path against a header sox
// wrote, so the GUID handling is validated against something this package did
// not produce. The 24- and 32-bit fixtures are extensible.
func TestWAVExtensibleFromSox(t *testing.T) {
	for _, name := range []string{"tone-i24le.wav", "tone-i32le.wav"} {
		raw, err := readFixture(name)
		if err != nil {
			t.Fatal(err)
		}
		if tag := binary.LittleEndian.Uint16(raw[20:22]); tag != wavExtensible {
			t.Fatalf("%s has tag %#04x; the fixture is meant to be extensible", name, tag)
		}
		if _, err := NewReader(bytes.NewReader(raw), Options{}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestSniffFallsBackToRaw is what lets one flag set read either format: a stream
// that is not a WAV must be readable as raw with nothing consumed.
func TestSniffFallsBackToRaw(t *testing.T) {
	raw := i16(0.5, -0.5, 0.25)
	r, err := NewReader(bytes.NewReader(raw), Options{Format: I16LE, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	got := readAll(t, r, 4)
	if len(got) != 3 || got[0] != 0.5 {
		t.Errorf("read %v, want the three samples back with nothing eaten by the sniff", got)
	}

	// Shorter than the four magic bytes.
	short, err := NewReader(bytes.NewReader([]byte{1, 2}), Options{Format: I8, Rate: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(readAll(t, short, 4)); n != 2 {
		t.Errorf("read %d samples from a 2-byte stream, want 2", n)
	}
}

func readFixture(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join("testdata", name))
}

// FuzzReadWAVHeader is the reason a byte parser gets a fuzz target: a file off
// the internet must not be able to panic it, and a parse that succeeds must
// describe something readable.
func FuzzReadWAVHeader(f *testing.F) {
	good := pcmFmt(wavPCM, 1, 8000, 16)
	payload := i16(0.5, -0.5)
	f.Add(wav(good, uint32(len(payload)), payload))
	f.Add(wav(good, 0xffffffff, payload))
	f.Add(wav(extFmt(1, 8000, 24, 20, ksGUID(wavPCM)), 6, make([]byte, 6)))
	f.Add([]byte("RIFF"))
	f.Add([]byte("RF64\xff\xff\xff\xffWAVE"))
	if raw, err := readFixture("tone-i16le.wav"); err == nil {
		f.Add(raw)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		r, err := NewReader(bytes.NewReader(in), Options{})
		if err != nil {
			return
		}
		info := r.Info()
		if info.Container != "wav" {
			// It fell through to raw, which needs a format and a rate this
			// caller did not supply, so it cannot have succeeded.
			t.Fatalf("a stream parsed as %q with no format given", info.Container)
		}
		if info.Channels <= 0 || info.Rate <= 0 || !info.Format.Valid() {
			t.Fatalf("parsed to an unusable Info: %+v", info)
		}
		if info.Frames < -1 {
			t.Fatalf("Frames = %d", info.Frames)
		}
		// Reading must terminate and must not panic.
		buf := make([]float64, 32)
		for range 64 {
			if _, err := r.ReadFloat64(buf); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				break
			}
		}
	})
}

func ksGUID(tag uint16) []byte {
	g := make([]byte, 16)
	binary.LittleEndian.PutUint16(g, tag)
	copy(g[2:], ksSuffix[:])
	return g
}

// dataBeforeFmt writes the data chunk ahead of the format chunk, which no
// writer does and which the walk has to reject rather than decode blind.
func dataBeforeFmt(fmtChunk []byte, payload []byte) []byte {
	body := new(bytes.Buffer)
	body.Write(chunk("data", payload))
	body.Write(chunk("fmt ", fmtChunk))

	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(4+body.Len()))
	b.WriteString("WAVE")
	b.Write(body.Bytes())
	return b.Bytes()
}

// shortCBSize rewrites the cbSize field of an extensible format chunk, leaving
// the rest of it intact, so the only thing wrong with the file is the count.
func shortCBSize(fmtChunk []byte, cbSize uint16) []byte {
	b := append([]byte(nil), fmtChunk...)
	binary.LittleEndian.PutUint16(b[16:], cbSize)
	return b
}

// rf64 builds an RF64 file whose ds64 chunk is ds64Size bytes long. Anything
// below 16 stops short of the 64-bit data size, so the 32-bit one stands.
func rf64(fmtChunk []byte, dataSize uint32, payload []byte, ds64Size int) []byte {
	ds := make([]byte, ds64Size)
	if ds64Size >= 8 {
		binary.LittleEndian.PutUint64(ds[0:], uint64(len(payload))+36) // riff size
	}
	if ds64Size >= 16 {
		binary.LittleEndian.PutUint64(ds[8:], uint64(len(payload))) // data size
	}

	body := new(bytes.Buffer)
	body.Write(chunk("ds64", ds))
	body.Write(chunk("fmt ", fmtChunk))
	body.WriteString("data")
	_ = binary.Write(body, binary.LittleEndian, dataSize)
	body.Write(payload)

	var b bytes.Buffer
	b.WriteString("RF64")
	_ = binary.Write(&b, binary.LittleEndian, uint32(0xffffffff))
	b.WriteString("WAVE")
	b.Write(body.Bytes())
	return b.Bytes()
}

// truncateBy drops the last n bytes, which is how a chunk header comes to
// promise more than the file holds.
func truncateBy(b []byte, n int) []byte {
	if n >= len(b) {
		return nil
	}
	return b[:len(b)-n]
}
