package sampleio

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"samuelks.com/dsp/pcm"
	"samuelks.com/dsp/riff"
)

// WAV format tags. A file with any other tag is not decoded.
const (
	wavPCM        = 0x0001
	wavFloat      = 0x0003
	wavALaw       = 0x0006
	wavMuLaw      = 0x0007
	wavExtensible = 0xfffe
)

// The sizes of a format chunk: plain PCM, and WAVE_FORMAT_EXTENSIBLE, the
// largest the format defines.
const (
	fmtSize    = 16
	fmtExtSize = 40
)

// ds64Size is the size of the data in RF64's ds64 chunk when it has no table:
// three 64-bit sizes (RIFF, data, frames) and a zero table length. A Writer
// reserves the space with a JUNK chunk of this size.
const ds64Size = 28

// Limits on the chunks a Reader keeps for Chunks. Chunk sizes come from the
// file and may be wrong or hostile, so both one chunk and the total are capped.
const (
	maxChunkSize  = 1 << 20
	maxChunkTotal = 4 << 20
)

// Speaker positions for Layout.ChannelMask, one bit each. Channels are
// interleaved in the order of the bits that are set.
const (
	SpeakerFrontLeft uint32 = 1 << iota
	SpeakerFrontRight
	SpeakerFrontCenter
	SpeakerLowFrequency
	SpeakerBackLeft
	SpeakerBackRight
	SpeakerFrontLeftOfCenter
	SpeakerFrontRightOfCenter
	SpeakerBackCenter
	SpeakerSideLeft
	SpeakerSideRight
	SpeakerTopCenter
	SpeakerTopFrontLeft
	SpeakerTopFrontCenter
	SpeakerTopFrontRight
	SpeakerTopBackLeft
	SpeakerTopBackCenter
	SpeakerTopBackRight
)

// The masks an extensible header gets when the caller gives none.
const (
	maskMono   = SpeakerFrontCenter
	maskStereo = SpeakerFrontLeft | SpeakerFrontRight
)

var (
	fourWAVE = riff.NewFourCC("WAVE")
	fourFmt  = riff.NewFourCC("fmt ")
	fourFact = riff.NewFourCC("fact")
	fourData = riff.NewFourCC("data")
	fourDS64 = riff.NewFourCC("ds64")
	fourJUNK = riff.NewFourCC("JUNK")
)

// ksSuffix is the last 14 bytes of a KSDATAFORMAT_SUBTYPE GUID, whose first
// two bytes are an ordinary format tag. A GUID with a different ending names
// some other codec.
var ksSuffix = [14]byte{0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}

// Chunk is a chunk of a WAV file that is neither the format nor the samples,
// such as LIST, bext or cue.
type Chunk struct {
	ID riff.FourCC

	// Size is the declared size of the chunk's data.
	Size uint32

	// Data is the chunk's contents, or nil when the chunk was too large to
	// keep and was skipped.
	Data []byte
}

// wavFmt is a WAV format chunk, used by both the Reader and the Writer. For
// an extensible chunk, tag is the one its SubFormat GUID names.
type wavFmt struct {
	tag       uint16
	channels  int
	rate      uint32
	bits      int    // container bits per sample
	validBits int    // significant bits; zero when that is all of them
	mask      uint32 // speaker assignment; zero when unspecified
}

// blockAlign is the size of one frame in bytes.
func (f wavFmt) blockAlign() int { return f.channels * f.bits / 8 }

// extensible reports whether the format needs a WAVE_FORMAT_EXTENSIBLE chunk.
// It does when it has more than two channels, a speaker mask, fewer valid bits
// than container bits, or PCM wider than 16 bits.
func (f wavFmt) extensible() bool {
	return f.channels > 2 || f.mask != 0 ||
		(f.validBits != 0 && f.validBits != f.bits) ||
		(f.tag == wavPCM && f.bits > 16)
}

// marshal encodes the format chunk. It is 16 bytes for PCM, 18 for any other
// plain format (which has an empty extension) and 40 for an extensible one. An
// extensible mono or stereo chunk with no mask is given the usual mask.
func (f wavFmt) marshal() []byte {
	le := binary.LittleEndian
	tag := f.tag
	size := fmtSize
	switch {
	case f.extensible():
		tag = wavExtensible
		size = fmtExtSize
	case f.tag != wavPCM:
		size = fmtSize + 2
	}
	b := make([]byte, size)
	le.PutUint16(b[0:], tag)
	le.PutUint16(b[2:], uint16(f.channels))
	le.PutUint32(b[4:], f.rate)
	le.PutUint32(b[8:], f.rate*uint32(f.blockAlign()))
	le.PutUint16(b[12:], uint16(f.blockAlign()))
	le.PutUint16(b[14:], uint16(f.bits))
	if tag == wavExtensible {
		valid, mask := f.validBits, f.mask
		if valid == 0 {
			valid = f.bits
		}
		if mask == 0 {
			switch f.channels {
			case 1:
				mask = maskMono
			case 2:
				mask = maskStereo
			}
		}
		le.PutUint16(b[16:], fmtExtSize-fmtSize-2)
		le.PutUint16(b[18:], uint16(valid))
		le.PutUint32(b[20:], mask)
		le.PutUint16(b[24:], f.tag)
		copy(b[26:], ksSuffix[:])
	}
	return b
}

// wavHeader is what was read from a WAV header.
type wavHeader struct {
	wavFmt
	format    pcm.Format
	dataBytes int64 // -1 when the size cannot be trusted; read to EOF
	chunks    []Chunk
	notes     []string
}

// sniffWAV looks for a WAV header at the start of the stream and parses it.
//
// It returns nil when the stream does not start with one, so the caller can
// read the stream as raw samples. Looking ahead needs a bufio.Reader, and the
// bytes it has buffered must not be lost, so sniffWAV replaces *src with that
// reader.
func sniffWAV(src *io.Reader) (*wavHeader, error) {
	br, ok := (*src).(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(*src, blockBytes)
	}
	*src = br

	magic, err := br.Peek(4)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// Too short to be a WAV. The caller decides whether it is a
			// valid raw stream.
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

// readWAVHeader reads the chunks up to the data chunk and leaves the reader
// at the first sample byte.
func readWAVHeader(r io.Reader) (*wavHeader, error) {
	rr, err := riff.New(r)
	if err != nil {
		return nil, fmt.Errorf("sampleio: reading the RIFF header: %w", err)
	}
	if rr.FormType != fourWAVE {
		return nil, fmt.Errorf("sampleio: %q is a RIFF file but not a WAVE", rr.FormType)
	}
	// The container's size is ignored. A streaming writer often gets it
	// wrong, and it is not needed: the walk stops at the data chunk, and the
	// Reader checks that chunk's size against the file.
	rr.IgnoreSize = true
	rr.AllowUnsizedChunk = fourData

	h := &wavHeader{dataBytes: -1}
	var (
		haveFmt  bool
		ds64Data int64 = -1
		kept     int64
	)
	for {
		c, err := rr.Next()
		if err != nil {
			return nil, headerErr(err)
		}

		switch c.ID {
		case fourData:
			if !haveFmt {
				return nil, errors.New("sampleio: the data chunk comes before the format chunk")
			}
			// A streaming writer does not know this size either. Three
			// values mean "to the end of the file": the all-ones placeholder,
			// zero, and the all-ones field of an RF64 whose real size is in
			// ds64.
			switch {
			case rr.RF64 && ds64Data >= 0:
				h.dataBytes = ds64Data
			case c.Size == 0xffffffff:
				h.notes = append(h.notes, "the data chunk size is a placeholder; reading to the end")
			case c.Size == 0:
				h.notes = append(h.notes, "the data chunk size is zero; reading to the end")
			default:
				h.dataBytes = int64(c.Size)
			}
			return h, nil

		case fourFmt:
			if c.Size < fmtSize {
				return nil, fmt.Errorf("sampleio: the format chunk is %d bytes, too short to read", c.Size)
			}
			var b [fmtExtSize]byte
			n, err := chunkHead(c, b[:])
			if err != nil {
				return nil, fmt.Errorf("sampleio: reading the format chunk: %w", err)
			}
			if err := h.parseFmt(b[:n]); err != nil {
				return nil, err
			}
			haveFmt = true

		case fourDS64:
			// RF64's 64-bit sizes, which replace the 32-bit ones.
			var b [ds64Size]byte
			n, err := chunkHead(c, b[:])
			if err != nil {
				return nil, fmt.Errorf("sampleio: reading the ds64 chunk: %w", err)
			}
			if n >= 16 {
				ds64Data = int64(binary.LittleEndian.Uint64(b[8:16]))
			}

		default:
			// Any other chunk, such as LIST, fact, PEAK, bext or id3, is kept
			// for Chunks.
			ch := Chunk{ID: c.ID, Size: c.Size}
			if size := int64(c.Size); size <= maxChunkSize && kept+size <= maxChunkTotal {
				if ch.Data, err = c.Bytes(maxChunkSize); err != nil {
					return nil, headerErr(err)
				}
				kept += size
			}
			h.chunks = append(h.chunks, ch)
		}
	}
}

// chunkHead reads the start of a chunk into b and returns how many bytes it
// read. The walk skips the rest of the chunk.
func chunkHead(c *riff.Chunk, b []byte) (int, error) {
	n := int(min(int64(c.Size), int64(len(b))))
	return io.ReadFull(c, b[:n])
}

// headerErr wraps an error from reading the header's chunks.
func headerErr(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return errors.New("sampleio: the file ended before its data chunk")
	}
	return fmt.Errorf("sampleio: reading the WAV header: %w", err)
}

// parseFmt parses a format chunk. b holds up to its first 40 bytes.
func (h *wavHeader) parseFmt(b []byte) error {
	le := binary.LittleEndian
	f := wavFmt{
		tag:      le.Uint16(b[0:2]),
		channels: int(le.Uint16(b[2:4])),
		rate:     le.Uint32(b[4:8]),
		bits:     int(le.Uint16(b[14:16])),
	}
	blockAlign := int(le.Uint16(b[12:14]))

	if f.tag == wavExtensible {
		if len(b) < fmtExtSize {
			return fmt.Errorf("sampleio: an extensible format chunk needs 40 bytes and has %d", len(b))
		}
		cbSize := int(le.Uint16(b[16:18]))
		if cbSize < 22 {
			return fmt.Errorf("sampleio: an extensible format chunk needs 22 extra bytes and declares %d", cbSize)
		}
		if v := int(le.Uint16(b[18:20])); v > 0 && v < f.bits {
			// The valid bits are the high bits of the container, so full
			// scale is still that of the container's width.
			f.validBits = v
		}
		f.mask = le.Uint32(b[20:24])
		guid := b[24:40]
		if !bytes.Equal(guid[2:], ksSuffix[:]) {
			return fmt.Errorf("sampleio: the format is a GUID this package does not decode (%x)", guid)
		}
		f.tag = le.Uint16(guid[0:2])
	}

	if f.channels <= 0 {
		return fmt.Errorf("sampleio: the header says %d channels", f.channels)
	}
	if f.rate == 0 {
		return errors.New("sampleio: the header says a sample rate of zero")
	}
	if f.bits <= 0 || f.bits%8 != 0 {
		return fmt.Errorf("sampleio: %d bits per sample is not a whole number of bytes", f.bits)
	}

	format, err := wavFormat(f.tag, f.bits)
	if err != nil {
		return err
	}
	h.wavFmt = f
	h.format = format

	if want := f.blockAlign(); blockAlign != want {
		// Some writers state the wrong block alignment but lay the samples
		// out correctly. Use the computed value and note the one ignored.
		h.notes = append(h.notes, fmt.Sprintf(
			"the header says a block of %d bytes; %d channels of %d bits is %d", blockAlign, f.channels, f.bits, want))
	}
	return nil
}

// wavFormat returns the encoding for a format tag and sample width. RIFF is
// little-endian, so the result is never a big-endian format.
func wavFormat(tag uint16, bits int) (pcm.Format, error) {
	switch tag {
	case wavPCM:
		switch bits {
		// WAV PCM is unsigned at 8 bits and signed at every wider width.
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

	case wavALaw:
		if bits == 8 {
			return pcm.ALaw, nil
		}
		return 0, fmt.Errorf("sampleio: %d-bit A-law", bits)

	case wavMuLaw:
		if bits == 8 {
			return pcm.MuLaw, nil
		}
		return 0, fmt.Errorf("sampleio: %d-bit mu-law", bits)
	}
	return 0, fmt.Errorf("sampleio: WAV format tag %#04x is a codec this package does not decode", tag)
}

// wavTag returns the WAV format tag for an encoding.
func wavTag(f pcm.Format) uint16 {
	switch {
	case f == pcm.ALaw:
		return wavALaw
	case f == pcm.MuLaw:
		return wavMuLaw
	case f.Float():
		return wavFloat
	}
	return wavPCM
}

// wavLayout holds the offsets of the header fields a Writer fills in on
// Close. junk and fact are -1 when the header has no such chunk.
type wavLayout struct {
	size int // header bytes, up to the first sample
	junk int // the JUNK chunk's id
	fact int // the fact chunk's frame count
	data int // the data chunk's size
}

// wavHeaderBytes builds a WAV header with placeholder sizes. When junk is set
// it includes a JUNK chunk that Close can turn into ds64.
//
// The format chunk is plain PCM for U8 and I16 with one or two channels and
// no mask. I24, I32, more than two channels or a mask make it extensible. A
// float or G.711 format uses its own tag, or the extensible form of it, and
// gets the fact chunk that every format but PCM requires.
func wavHeaderBytes(f pcm.Format, channels int, rate, mask uint32, junk bool) ([]byte, wavLayout, error) {
	l := wavLayout{junk: -1, fact: -1}
	wf := wavFmt{tag: wavTag(f), channels: channels, rate: rate, bits: f.Width() * 8, mask: mask}

	// The header is built in a buffer, which cannot seek, so riff leaves the
	// container and data sizes as placeholders. Close fills them in for a
	// file.
	var b bytes.Buffer
	w, err := riff.NewWriter(&b, fourWAVE)
	if err != nil {
		return nil, l, err
	}
	if junk {
		l.junk = b.Len()
		if err := w.WriteChunk(fourJUNK, make([]byte, ds64Size)); err != nil {
			return nil, l, err
		}
	}
	if err := w.WriteChunk(fourFmt, wf.marshal()); err != nil {
		return nil, l, err
	}
	if wf.tag != wavPCM {
		if err := w.WriteChunk(fourFact, make([]byte, 4)); err != nil {
			return nil, l, err
		}
		l.fact = b.Len() - 4
	}
	if _, err := w.Chunk(fourData); err != nil {
		return nil, l, err
	}
	l.data = b.Len() - 4
	l.size = b.Len()
	return b.Bytes(), l, nil
}
