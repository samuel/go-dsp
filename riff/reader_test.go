package riff

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func chunk(id string, data []byte) []byte {
	b := []byte(id)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, data...)
	if len(data)%2 != 0 {
		b = append(b, 0)
	}
	return b
}

func container(form string, body []byte) []byte {
	b := []byte("RIFF")
	b = binary.LittleEndian.AppendUint32(b, uint32(4+len(body)))
	b = append(b, form...)
	return append(b, body...)
}

func TestChunkIteration(t *testing.T) {
	body := append(chunk("fmt ", []byte("0123456789abcdef")), chunk("odd ", []byte("abc"))...)
	body = append(body, chunk("data", []byte("xyzw"))...)
	file := container("WAVE", body)

	r, err := New(bytes.NewReader(file))
	if err != nil || r.FormType.String() != "WAVE" {
		t.Fatalf("New: %v %q", err, r.FormType)
	}
	var first *Chunk
	var ids []string
	for {
		c, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID.String())
		b, err := c.Bytes(64)
		if err != nil {
			t.Fatal(err)
		}
		if c.ID.String() == "odd " && string(b) != "abc" {
			t.Fatalf("odd data %q", b)
		}
		if first == nil {
			first = c
		}
	}
	if len(ids) != 3 || ids[2] != "data" {
		t.Fatalf("ids %v", ids)
	}
	// stale chunk
	if _, err := first.Read(make([]byte, 4)); !errors.Is(err, ErrStaleChunk) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := first.Bytes(4); !errors.Is(err, ErrStaleChunk) {
		t.Fatalf("stale bytes: %v", err)
	}
}

func TestTruncatedChunk(t *testing.T) {
	file := container("WAVE", chunk("data", bytes.Repeat([]byte("a"), 20)))
	r, _ := New(bytes.NewReader(file[:len(file)-5]))
	c, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(c); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("R2 read: %v", err)
	}
	r2, _ := New(bytes.NewReader(file[:len(file)-5]))
	if _, err := r2.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err := r2.Next(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("R2 skip: %v", err)
	}
}

func TestChunkSizeBeyondContainer(t *testing.T) {
	// chunk claims more than the container holds
	body := chunk("data", []byte("abcd"))
	binary.LittleEndian.PutUint32(body[4:8], 1<<20)
	file := container("WAVE", body)
	r, _ := New(bytes.NewReader(file))
	if _, err := r.Next(); !errors.Is(err, ErrBadChunkSize) {
		t.Fatalf("R3: %v", err)
	}
}

func TestListNesting(t *testing.T) {
	inner := append([]byte("INFO"), chunk("INAM", []byte("name"))...)
	inner = append(inner, chunk("IART", []byte("artist"))...)
	file := container("WAVE", append(chunk("LIST", inner), chunk("data", []byte("ab"))...))
	r, _ := New(bytes.NewReader(file))
	c, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	sub, err := c.List()
	if err != nil || sub.FormType.String() != "INFO" {
		t.Fatalf("List: %v %q", err, sub.FormType)
	}
	var got []string
	for {
		sc, err := sub.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := sc.Bytes(32)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, sc.ID.String()+"="+string(b))
	}
	if len(got) != 2 || got[0] != "INAM=name" || got[1] != "IART=artist" {
		t.Fatalf("list chunks %v", got)
	}
	d, err := r.Next()
	if err != nil || d.ID.String() != "data" {
		t.Fatalf("after list: %v %v", err, d)
	}
	if _, err := sub.Next(); !errors.Is(err, ErrStaleChunk) {
		t.Fatalf("stale sublist: %v", err)
	}
}

func TestUnsizedFinalChunk(t *testing.T) {
	body := chunk("data", []byte("abcdef"))
	binary.LittleEndian.PutUint32(body[4:8], 0xFFFFFFFF)
	file := []byte("RIFF")
	file = binary.LittleEndian.AppendUint32(file, 0xFFFFFFFF)
	file = append(file, "WAVE"...)
	file = append(file, body...)

	r, _ := New(bytes.NewReader(file))
	if _, err := r.Next(); !errors.Is(err, ErrBadChunkSize) {
		t.Fatalf("R5 default: %v", err)
	}
	r2, _ := New(bytes.NewReader(file))
	r2.AllowUnsizedChunk = NewFourCC("data")
	c, err := r2.Next()
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Bytes(32)
	if err != nil || string(b) != "abcdef" {
		t.Fatalf("unsized: %v %q", err, b)
	}
	if _, err := r2.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after unsized: %v", err)
	}
}

func TestEmptyChunkInSizedContainer(t *testing.T) {
	body := append(chunk("LIST", nil), chunk("data", []byte("abcdef"))...)
	file := container("WAVE", body)

	// The flag must not turn an empty chunk in a sized container into one that
	// swallows the rest of the stream.
	r, _ := New(bytes.NewReader(file))
	r.AllowUnsizedChunk = NewFourCC("data")
	c, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if c.Unsized || c.Size != 0 {
		t.Fatalf("LIST: unsized=%v size=%d", c.Unsized, c.Size)
	}
	if b, err := c.Bytes(8); err != nil || len(b) != 0 {
		t.Fatalf("LIST data: %v %q", err, b)
	}
	d, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Bytes(8)
	if err != nil || d.ID.String() != "data" || string(b) != "abcdef" {
		t.Fatalf("data: %v %q %q", err, d.ID, b)
	}
}

func TestUnsizedChunkInSizedContainer(t *testing.T) {
	body := chunk("data", []byte("abcdef"))
	binary.LittleEndian.PutUint32(body[4:8], 0xFFFFFFFF)
	r, _ := New(bytes.NewReader(container("WAVE", body)))
	r.AllowUnsizedChunk = NewFourCC("data")
	if _, err := r.Next(); !errors.Is(err, ErrBadChunkSize) {
		t.Fatalf("expected ErrBadChunkSize, got %v", err)
	}
}

func TestNotRIFF(t *testing.T) {
	if _, err := New(bytes.NewReader([]byte("JUNKxxxxWAVE"))); !errors.Is(err, ErrNotRIFF) {
		t.Fatal(err)
	}
	if _, err := New(bytes.NewReader([]byte("RIF"))); !errors.Is(err, ErrNotRIFF) {
		t.Fatal(err)
	}
}

// Only the named chunk may run to EOF; an empty chunk ahead of it keeps its
// declared size instead of swallowing the rest of the stream.
func TestUnsizedChunkOnlyAppliesToNamedChunk(t *testing.T) {
	body := append(chunk("JUNK", nil), chunk("fmt ", []byte("0123456789abcdef"))...)
	unsizedData := chunk("data", []byte("abcdef"))
	binary.LittleEndian.PutUint32(unsizedData[4:8], 0xFFFFFFFF)
	body = append(body, unsizedData...)

	file := []byte("RIFF")
	file = binary.LittleEndian.AppendUint32(file, 0xFFFFFFFF)
	file = append(file, "WAVE"...)
	file = append(file, body...)

	r, err := New(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	r.AllowUnsizedChunk = NewFourCC("data")

	var ids []string
	for {
		c, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID.String())
		if c.ID.String() == "JUNK" && c.Unsized {
			t.Error("JUNK swallowed the stream")
		}
		if c.ID.String() == "data" {
			if !c.Unsized {
				t.Error("data is not unsized")
			}
			b, err := c.Bytes(32)
			if err != nil || string(b) != "abcdef" {
				t.Fatalf("data: %v %q", err, b)
			}
		}
	}
	if want := []string{"JUNK", "fmt ", "data"}; len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Errorf("chunks = %q, want %q", ids, want)
	}
}

// A chunk that is not the named one keeps reporting an all-ones size as an
// error, whatever the container says.
func TestUnsizedChunkWrongIDRejected(t *testing.T) {
	body := chunk("JUNK", []byte("abcdef"))
	binary.LittleEndian.PutUint32(body[4:8], 0xFFFFFFFF)
	file := []byte("RIFF")
	file = binary.LittleEndian.AppendUint32(file, 0xFFFFFFFF)
	file = append(file, "WAVE"...)
	file = append(file, body...)

	r, _ := New(bytes.NewReader(file))
	r.AllowUnsizedChunk = NewFourCC("data")
	if _, err := r.Next(); !errors.Is(err, ErrBadChunkSize) {
		t.Fatalf("JUNK with an all-ones size = %v, want ErrBadChunkSize", err)
	}
}

// TestRF64 reads the 64-bit form as an unsized container whose ds64 chunk is
// an ordinary chunk.
func TestRF64(t *testing.T) {
	body := append(chunk("ds64", make([]byte, 28)), chunk("fmt ", []byte("0123456789abcdef"))...)
	body = append(body, "data\xff\xff\xff\xffxyzw"...)
	file := append([]byte("RF64\xff\xff\xff\xffWAVE"), body...)

	r, err := New(bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	if !r.RF64 || r.FormType.String() != "WAVE" {
		t.Fatalf("RF64 = %v, FormType = %q", r.RF64, r.FormType)
	}
	r.AllowUnsizedChunk = NewFourCC("data")
	var ids []string
	for {
		c, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID.String())
		if c.ID.String() == "data" {
			b, err := c.Bytes(64)
			if err != nil || !c.Unsized || string(b) != "xyzw" {
				t.Fatalf("data %q, unsized %v: %v", b, c.Unsized, err)
			}
		}
	}
	if len(ids) != 3 || ids[0] != "ds64" || ids[2] != "data" {
		t.Fatalf("ids %v", ids)
	}

	plain, err := New(bytes.NewReader(container("WAVE", nil)))
	if err != nil || plain.RF64 {
		t.Fatalf("a RIFF container reports RF64 = %v: %v", plain.RF64, err)
	}
}

// TestIgnoreSize reads chunks a sized container says it has no room for.
func TestIgnoreSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dataSize uint32
		unsized  bool
	}{
		{"overstated", 1 << 20, false},
		{"placeholder", 0xffffffff, true},
		{"zero", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := chunk("fmt ", []byte("0123456789abcdef"))
			body = append(body, "data"...)
			body = binary.LittleEndian.AppendUint32(body, tc.dataSize)
			body = append(body, "xyzw"...)
			file := container("WAVE", body)

			strict, err := New(bytes.NewReader(file))
			if err != nil {
				t.Fatal(err)
			}
			strict.AllowUnsizedChunk = NewFourCC("data")
			if _, err := strict.Next(); err != nil {
				t.Fatal(err)
			}
			if c, err := strict.Next(); tc.dataSize != 0 && !errors.Is(err, ErrBadChunkSize) {
				t.Fatalf("a sized container accepted the chunk: %+v, %v", c, err)
			}

			r, err := New(bytes.NewReader(file))
			if err != nil {
				t.Fatal(err)
			}
			r.AllowUnsizedChunk = NewFourCC("data")
			r.IgnoreSize = true
			if _, err := r.Next(); err != nil {
				t.Fatal(err)
			}
			c, err := r.Next()
			if err != nil {
				t.Fatal(err)
			}
			if c.Unsized != tc.unsized {
				t.Errorf("Unsized = %v, want %v", c.Unsized, tc.unsized)
			}
			var got [4]byte
			if _, err := io.ReadFull(c, got[:]); err != nil || string(got[:]) != "xyzw" {
				t.Errorf("read %q: %v", got, err)
			}
		})
	}
}
