package sampleio

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
	format    Format
	rate      float64
	channels  int
	bits      int
	validBits int
	codec     string
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

	f, codec, err := wavFormat(tag, bits)
	if err != nil {
		return err
	}
	h.format, h.codec = f, codec
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
func wavFormat(tag uint16, bits int) (Format, string, error) {
	switch tag {
	case wavPCM:
		switch bits {
		// 8-bit WAV PCM is unsigned, every wider width signed. It is a wart in
		// the format, and the reason an 8-bit file read as signed looks
		// inverted.
		case 8:
			return U8, "pcm", nil
		case 16:
			return I16LE, "pcm", nil
		case 24:
			return I24LE, "pcm", nil
		case 32:
			return I32LE, "pcm", nil
		}
		return 0, "", fmt.Errorf("sampleio: %d-bit PCM", bits)

	case wavFloat:
		switch bits {
		case 32:
			return F32LE, "float", nil
		case 64:
			return F64LE, "float", nil
		}
		return 0, "", fmt.Errorf("sampleio: %d-bit float", bits)

	case wavALaw, wavMuLaw:
		name := "A-law"
		if tag == wavMuLaw {
			name = "mu-law"
		}
		return 0, "", fmt.Errorf("sampleio: %s is a companded format this package does not decode yet", name)
	}
	return 0, "", fmt.Errorf("sampleio: WAV format tag %#04x is a codec this package does not decode", tag)
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
