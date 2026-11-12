package sampleio

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"samuelks.com/dsp/pcm"
)

// WAV format tags. Anything else is a codec this package does not decode.
const (
	wavPCM        = 0x0001
	wavFloat      = 0x0003
	wavALaw       = 0x0006
	wavMuLaw      = 0x0007
	wavExtensible = 0xfffe
)

// ksSuffix is the fixed tail of a KSDATAFORMAT_SUBTYPE GUID. The first two
// bytes carry the real format tag; a GUID that does not end this way is a codec
// of its own, not a PCM tag in disguise.
var ksSuffix = [14]byte{0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}

// wavHeader is what a WAV says about itself.
type wavHeader struct {
	format    pcm.Format
	rate      float64
	channels  int
	bits      int
	validBits int
	dataBytes int64 // -1 when the size cannot be trusted; read to EOF
	notes     []string
}

// sniffWAV looks for a WAV header at the front of the stream.
//
// It returns nil without consuming anything if the stream does not start with
// one, so the caller can fall back to reading it raw; that is why it takes a
// pointer, because the bufio.Reader it wraps the source in must outlive the
// call for the buffered bytes not to be lost.
func sniffWAV(src *io.Reader) (*wavHeader, error) {
	br, ok := (*src).(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(*src, blockBytes)
	}
	*src = br

	magic, err := br.Peek(4)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Too short to be a WAV. Whether it is a valid raw stream is the
			// caller's question, not this one's.
			return nil, nil
		}
		return nil, err
	}
	switch string(magic) {
	case "RIFF", "RF64":
	default:
		return nil, nil
	}
	return readWAVHeader(br)
}

// readWAVHeader walks the chunks up to the data chunk and leaves the reader
// positioned at the first sample byte.
func readWAVHeader(r io.Reader) (*wavHeader, error) {
	var riff [12]byte
	if _, err := io.ReadFull(r, riff[:]); err != nil {
		return nil, fmt.Errorf("sampleio: reading the RIFF header: %w", err)
	}
	if string(riff[8:12]) != "WAVE" {
		return nil, fmt.Errorf("sampleio: %q is a RIFF file but not a WAVE", riff[8:12])
	}
	rf64 := string(riff[0:4]) == "RF64"

	h := &wavHeader{dataBytes: -1}
	var (
		haveFmt  bool
		ds64Data int64 = -1
	)

	for {
		var head [8]byte
		if _, err := io.ReadFull(r, head[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, errors.New("sampleio: the file ended before its data chunk")
			}
			return nil, err
		}
		id := string(head[0:4])
		size := int64(binary.LittleEndian.Uint32(head[4:8]))

		switch id {
		case "data":
			// The size is the first thing a streaming writer cannot know, so
			// three values all mean "to the end of the file": the placeholder a
			// pipe leaves, a zero, and RF64's 32-bit field that ds64 replaces.
			switch {
			case rf64 && ds64Data >= 0:
				h.dataBytes = ds64Data
			case size == 0xffffffff:
				h.notes = append(h.notes, "the data chunk size is a placeholder; reading to the end")
			case size == 0:
				h.notes = append(h.notes, "the data chunk size is zero; reading to the end")
			default:
				h.dataBytes = size
			}
			if !haveFmt {
				return nil, errors.New("sampleio: the data chunk comes before the format chunk")
			}
			return h, nil

		case "fmt ":
			if err := h.readFmt(r, size); err != nil {
				return nil, err
			}
			haveFmt = true

		case "ds64":
			// RF64's 64-bit sizes, which stand in for the 32-bit ones.
			var b [28]byte
			n := min(size, int64(len(b)))
			if _, err := io.ReadFull(r, b[:n]); err != nil {
				return nil, fmt.Errorf("sampleio: reading the ds64 chunk: %w", err)
			}
			if n >= 16 {
				ds64Data = int64(binary.LittleEndian.Uint64(b[8:16]))
			}
			if err := skipBytes(r, size-n); err != nil {
				return nil, err
			}

		default:
			// LIST, fact, PEAK, bext, id3 and whatever else; unknown is not the
			// same as wrong.
			if err := skipBytes(r, size); err != nil {
				return nil, err
			}
		}

		// Chunks are word-aligned: an odd size is followed by a pad byte the size
		// does not count. Forgetting it desynchronizes every chunk after the
		// first odd one.
		if size%2 == 1 {
			if err := skipBytes(r, 1); err != nil {
				return nil, err
			}
		}
	}
}

// readFmt parses a format chunk.
func (h *wavHeader) readFmt(r io.Reader, size int64) error {
	if size < 16 {
		return fmt.Errorf("sampleio: the format chunk is %d bytes, too short to read", size)
	}
	var b [40]byte
	n := min(size, int64(len(b)))
	if _, err := io.ReadFull(r, b[:n]); err != nil {
		return fmt.Errorf("sampleio: reading the format chunk: %w", err)
	}

	tag := binary.LittleEndian.Uint16(b[0:2])
	channels := int(binary.LittleEndian.Uint16(b[2:4]))
	rate := binary.LittleEndian.Uint32(b[4:8])
	blockAlign := int(binary.LittleEndian.Uint16(b[12:14]))
	bits := int(binary.LittleEndian.Uint16(b[14:16]))

	if tag == wavExtensible {
		if n < 40 {
			return fmt.Errorf("sampleio: an extensible format chunk needs 40 bytes and has %d", n)
		}
		cbSize := int(binary.LittleEndian.Uint16(b[16:18]))
		if cbSize < 22 {
			return fmt.Errorf("sampleio: an extensible format chunk needs 22 extra bytes and declares %d", cbSize)
		}
		if v := int(binary.LittleEndian.Uint16(b[18:20])); v > 0 && v < bits {
			// The sample is left-justified in its container, so the container's
			// width is still what full scale is measured against.
			h.validBits = v
		}
		guid := b[24:40]
		if [14]byte(guid[2:16]) != ksSuffix {
			return fmt.Errorf("sampleio: the format is a GUID this package does not decode (%x)", guid)
		}
		tag = binary.LittleEndian.Uint16(guid[0:2])
	}

	if channels <= 0 {
		return fmt.Errorf("sampleio: the header says %d channels", channels)
	}
	if rate == 0 {
		return errors.New("sampleio: the header says a sample rate of zero")
	}
	if bits <= 0 || bits%8 != 0 {
		return fmt.Errorf("sampleio: %d bits per sample is not a whole number of bytes", bits)
	}

	f, err := wavFormat(tag, bits)
	if err != nil {
		return err
	}
	h.format = f
	h.channels = channels
	h.rate = float64(rate)
	h.bits = bits

	if want := channels * bits / 8; blockAlign != want {
		// Some writers get block alignment wrong and lay the samples out
		// correctly anyway, so believe the arithmetic and say what was ignored.
		h.notes = append(h.notes, fmt.Sprintf(
			"the header says a block of %d bytes; %d channels of %d bits is %d", blockAlign, channels, bits, want))
	}

	return skipBytes(r, size-n)
}

// wavFormat maps a tag and a width to an encoding. RIFF is little-endian, so
// nothing here can be big-endian.
func wavFormat(tag uint16, bits int) (pcm.Format, error) {
	switch tag {
	case wavPCM:
		switch bits {
		// 8-bit WAV PCM is unsigned, every wider width signed. It is a wart in
		// the format, and the reason an 8-bit file read as signed looks
		// inverted.
		case 8:
			return pcm.U8, nil
		case 16:
			return pcm.I16LE, nil
		case 24:
			return pcm.I24LE, nil
		case 32:
			return pcm.I32LE, nil
		}
		return 0, fmt.Errorf("sampleio: %d-bit PCM", bits)

	case wavFloat:
		switch bits {
		case 32:
			return pcm.F32LE, nil
		case 64:
			return pcm.F64LE, nil
		}
		return 0, fmt.Errorf("sampleio: %d-bit float", bits)

	case wavALaw, wavMuLaw:
		name := "A-law"
		if tag == wavMuLaw {
			name = "mu-law"
		}
		return 0, fmt.Errorf("sampleio: %s is a companded format this package does not decode yet", name)
	}
	return 0, fmt.Errorf("sampleio: WAV format tag %#04x is a codec this package does not decode", tag)
}

// skipBytes discards n bytes.
func skipBytes(r io.Reader, n int64) error {
	if n <= 0 {
		return nil
	}
	if _, err := io.CopyN(io.Discard, r, n); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("sampleio: the file ended inside a chunk")
		}
		return err
	}
	return nil
}

// ds64Size is the payload of RF64's ds64 chunk with no table: the RIFF, data
// and frame counts at 64 bits and a zero table length. A Writer reserves it as
// a JUNK chunk of the same size.
const ds64Size = 28

// Speaker masks for WAVE_FORMAT_EXTENSIBLE: front center for mono, front left
// and right for stereo.
const (
	maskMono   = 0x4
	maskStereo = 0x3
)

// wavLayout is where the fields a Writer patches on Close sit in the header it
// wrote. junk and fact are -1 when the header has no such chunk.
type wavLayout struct {
	size int // header bytes, up to the first sample
	junk int // the JUNK chunk's id
	fact int // the fact chunk's frame count
	data int // the data chunk's size
}

// wavHeaderBytes builds a WAV header with the placeholder sizes a pipe reader
// expects, and a JUNK chunk to become ds64 when junk is set.
//
// U8 and I16 with one or two channels get the plain PCM chunk, I24, I32 and
// more than two channels WAVE_FORMAT_EXTENSIBLE, and a float format the IEEE
// float tag, or the extensible float subtype past two channels, with the fact
// chunk a float WAV requires.
func wavHeaderBytes(f pcm.Format, channels int, rate uint32, junk bool) ([]byte, wavLayout) {
	le := binary.LittleEndian
	l := wavLayout{junk: -1, fact: -1}
	b := []byte("RIFF")
	b = le.AppendUint32(b, math.MaxUint32)
	b = append(b, "WAVE"...)

	if junk {
		l.junk = len(b)
		b = append(b, "JUNK"...)
		b = le.AppendUint32(b, ds64Size)
		b = append(b, make([]byte, ds64Size)...)
	}

	tag := uint16(wavPCM)
	if f.Float() {
		tag = wavFloat
	}
	bits := f.Width() * 8
	block := channels * f.Width()
	extensible := f == pcm.I24LE || f == pcm.I32LE || channels > 2

	b = append(b, "fmt "...)
	switch {
	case extensible:
		b = le.AppendUint32(b, 40)
	case f.Float():
		b = le.AppendUint32(b, 18)
	default:
		b = le.AppendUint32(b, 16)
	}
	if extensible {
		b = le.AppendUint16(b, wavExtensible)
	} else {
		b = le.AppendUint16(b, tag)
	}
	b = le.AppendUint16(b, uint16(channels))
	b = le.AppendUint32(b, rate)
	b = le.AppendUint32(b, rate*uint32(block))
	b = le.AppendUint16(b, uint16(block))
	b = le.AppendUint16(b, uint16(bits))
	switch {
	case extensible:
		var mask uint32
		switch channels {
		case 1:
			mask = maskMono
		case 2:
			mask = maskStereo
		}
		b = le.AppendUint16(b, 22)
		b = le.AppendUint16(b, uint16(bits)) // valid bits
		b = le.AppendUint32(b, mask)
		b = le.AppendUint16(b, tag)
		b = append(b, ksSuffix[:]...)
	case f.Float():
		b = le.AppendUint16(b, 0)
	}

	if f.Float() {
		b = append(b, "fact"...)
		b = le.AppendUint32(b, 4)
		l.fact = len(b)
		b = le.AppendUint32(b, 0)
	}

	b = append(b, "data"...)
	l.data = len(b)
	b = le.AppendUint32(b, math.MaxUint32)
	l.size = len(b)
	return b, l
}
