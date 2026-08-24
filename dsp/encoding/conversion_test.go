package encoding

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

const benchSize = 1 << 14

// simdBenchSizes is the size sweep for functions whose loop overhead is only
// visible while the working set fits in L1 or L2; a single large buffer is
// already bandwidth-bound and cannot see it. 1K-1 is deliberately not a block
// multiple, so it exercises the vector tail.
var simdBenchSizes = []struct {
	name string
	n    int
}{
	{"64", 64}, {"256", 256}, {"1K", 1024}, {"1K-1", 1023},
	{"4K", 4096}, {"16K", 16384}, {"64K", 65536},
}

func TestU8ToI16(t *testing.T) {
	src := make([]byte, 300)
	for i := range src {
		src[i] = byte(i)
	}
	src = src[:256]
	dst := make([]int16, len(src)+8)
	expected := make([]int16, len(src)+8)
	u8ToI16(expected, src) // Use Go implementation as reference
	U8ToI16(dst, src)
	for i, v := range expected {
		if dst[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
		}
	}

	// Unaligned input
	src = src[1:]
	dst = make([]int16, len(src)+8)
	expected = make([]int16, len(src)+8)
	u8ToI16(expected, src) // Use Go implementation as reference
	U8ToI16(dst, src)
	for i, v := range expected {
		if dst[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
		}
	}
}

func TestU8ToI16LE(t *testing.T) {
	src := make([]byte, 300)
	for i := range src {
		src[i] = byte(i)
	}
	src = src[:256]
	dst := make([]byte, len(src)*2+16)
	expected := make([]byte, len(src)*2+16)
	u8ToI16LE(expected, src) // Use Go implementation as reference
	U8ToI16LE(dst, src)
	if !bytes.Equal(dst, expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
	}

	// Make sure unmatched input and output lengths don't cause a panic/segfault
	U8ToI16LE([]byte{1, 2}, []byte{})
	U8ToI16LE([]byte{1, 2}, []byte{1, 2})

	// Unaligned output (even), non 8-byte multiple input
	src = src[1:]
	dst = make([]byte, len(src)*2+16)[2:]
	expected = make([]byte, len(src)*2+16)[2:]
	u8ToI16LE(expected, src) // Use Go implementation as reference
	U8ToI16LE(dst, src)
	if !bytes.Equal(dst, expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
	}

	// Unaligned output (odd), non 8-byte multiple input
	src = src[1:]
	dst = make([]byte, len(src)*2+16)[1:]
	expected = make([]byte, len(src)*2+16)[1:]
	u8ToI16LE(expected, src) // Use Go implementation as reference
	U8ToI16LE(dst, src)
	if !bytes.Equal(dst, expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
	}
}

// u8ToF32Ref is the reference the U8ToF32 tests compare against, exactly.
// Every result is a small integer, so every implementation of this conversion
// is exact and there is nothing to allow a tolerance for.
func u8ToF32Ref(v byte) float32 { return float32(int(v) - 128) }

func TestU8ToF32(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		// Every length across a couple of unrolled blocks, so a mishandled tail
		// cannot hide. Past three full blocks of the widest implementation,
		// which is 64 samples on the amd64 512-bit path.
		for n := range 200 {
			src := make([]byte, n)
			expected := make([]float32, n)
			for i := range src {
				src[i] = byte(i * 7)
				expected[i] = u8ToF32Ref(src[i])
			}

			dst := make([]float32, n)
			U8ToF32(dst, src)
			for i, v := range expected {
				if dst[i] != v {
					t.Fatalf("len %d: dst[%d] = %v, want %v (src %d)", n, i, dst[i], v, src[i])
				}
			}

			// Unaligned input and output
			ui := make([]byte, n+1)[1:]
			copy(ui, src)
			uo := make([]float32, n+1)[1:]
			U8ToF32(uo, ui)
			for i, v := range expected {
				if uo[i] != v {
					t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, uo[i], v)
				}
			}
		}

		// Every byte value, so a botched sign extension of the 0x80..0xff half
		// cannot hide behind a narrow test input.
		all := make([]byte, 256)
		for i := range all {
			all[i] = byte(i)
		}
		out := make([]float32, 256)
		U8ToF32(out, all)
		for i, v := range all {
			if want := u8ToF32Ref(v); out[i] != want {
				t.Fatalf("src %d: got %v want %v", v, out[i], want)
			}
		}

		// Empty and nil slices, which the reference this replaced indexed out
		// of range.
		U8ToF32(nil, nil)
		U8ToF32([]float32{}, []byte{})

		// Mismatched lengths: only min(len(src), len(dst)) is touched.
		src := make([]byte, 64)
		for i := range src {
			src[i] = byte(i * 3)
		}
		for _, outLen := range []int{40, 64, 96} {
			dst := make([]float32, outLen)
			U8ToF32(dst, src)
			for i, v := range dst {
				var want float32
				if i < len(src) {
					want = u8ToF32Ref(src[i])
				}
				if v != want {
					t.Fatalf("src 64 dst %d: dst[%d] = %v, want %v", outLen, i, v, want)
				}
			}
		}
	})
}

func TestI8ToF32(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		src := make([]byte, 300)
		for i := range src {
			src[i] = byte(int8(i - 128))
		}
		src = src[:256]
		dst := make([]float32, len(src)+4)
		expected := make([]float32, len(src)+4)
		i8ToF32(expected, src) // Use Go implementation as reference
		I8ToF32(dst, src)
		for i := range dst {
			if dst[i] != expected[i] {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}

		// Unaligned
		src = src[1:]
		dst = make([]float32, len(src)+4)[1:]
		expected = make([]float32, len(src)+4)[1:]
		i8ToF32(expected, src) // Use Go implementation as reference
		I8ToF32(dst, src)
		for i := range dst {
			if dst[i] != expected[i] {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}
	})
}

// u8ToC64Ref is the reference the U8ToC64 tests compare against, exactly.
func u8ToC64Ref(dst []complex64, src []byte) {
	n := min(len(src)/2, len(dst))
	for i := range n {
		dst[i] = complex(u8ToF32Ref(src[i*2]), u8ToF32Ref(src[i*2+1]))
	}
}

func TestU8ToC64(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		check := func(name string, src []byte, dst, expected []complex64) {
			t.Helper()
			u8ToC64Ref(expected, src)
			U8ToC64(dst, src)
			for i := range dst {
				if dst[i] != expected[i] {
					t.Fatalf("%s: dst[%d] = %v, want %v", name, i, dst[i], expected[i])
				}
			}
		}

		// Every length across a couple of unrolled blocks, so a mishandled tail
		// cannot hide, and an odd input length on every one of them so the
		// truncation to whole complex values is exercised too.
		for n := range 200 {
			src := make([]byte, n)
			for i := range src {
				src[i] = byte(i * 7)
			}
			check("len", src, make([]complex64, n/2), make([]complex64, n/2))

			// Unaligned input and output
			ui := make([]byte, n+1)[1:]
			copy(ui, src)
			check("unaligned", ui, make([]complex64, n/2+1)[1:], make([]complex64, n/2+1)[1:])
		}

		// Every byte value, so a botched sign extension of the 0x80..0xff half
		// cannot hide.
		all := make([]byte, 256)
		for i := range all {
			all[i] = byte(i)
		}
		check("all bytes", all, make([]complex64, 128), make([]complex64, 128))

		// Empty and nil slices.
		U8ToC64(nil, nil)
		U8ToC64([]complex64{}, []byte{})

		// More input than output, and the reverse: only
		// min(len(src)/2, len(dst)) samples are touched either way.
		check("longer src", []byte{0, 1, 192, 200, 1, 2, 3, 4, 5, 6, 7},
			make([]complex64, 2), make([]complex64, 2))
		check("longer dst", []byte{0, 1, 192, 200},
			make([]complex64, 40), make([]complex64, 40))
	})
}

func TestF32ToI16(t *testing.T) {
	// Make sure there's non-zero value after the expected length of the slice
	// to detect out of bound access.
	src := make([]float32, 300)
	for i := range src {
		src[i] = 2.0*float32(i)/float32(len(src)) - 1.0
	}
	src = src[:256]
	dst := make([]int16, len(src)+4)
	expected := make([]int16, len(src)+4)
	f32ToI16(expected, src) // Use Go implementation as reference
	F32ToI16(dst, src)
	for i, v := range expected {
		if dst[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
		}
	}

	// Unaligned
	src = src[1:]
	dst = make([]int16, len(src)+4)[1:]
	expected = make([]int16, len(src)+4)[1:]
	f32ToI16(expected, src) // Use Go implementation as reference
	F32ToI16(dst, src)
	for i, v := range expected {
		if dst[i] != v {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
		}
	}
}

func TestF32ToI16LE(t *testing.T) {
	// Make sure there's non-zero value after the expected length of the slice
	// to detect out of bound access.
	src := []float32{0.0, 1.0, -1.0, 2.13, -2.13, 2.0, 3.0, 4.0}[:5]
	dst := make([]byte, len(src)*2+4)
	expected := make([]byte, len(src)*2+4)
	f32ToI16LE(expected, src) // Use Go implementation as reference
	F32ToI16LE(dst, src)
	if !bytes.Equal(dst, expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
	}
}

func TestI16LEToF64(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		src := []byte{
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x80, 0x00, 0x00, 0xff,
		}
		dst := make([]float64, len(src)/2)
		expected := make([]float64, len(dst))
		i16LEToF64(expected, src)
		I16LEToF64(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}
		i16LEToF64(expected, src)
		I16LEToF64(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}

		// Unaligned
		src = src[1 : len(src)-1]
		dst = make([]float64, len(src)/2+1)[1:]
		expected = make([]float64, len(dst))
		i16LEToF64(expected, src)
		I16LEToF64(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}

		// Make sure there's non-zero value after the expected length of the slice
		// to detect out of bound access.
		src = []byte{0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80, 0x64, 0x00, 0xff, 0x00}[:10]
		dst = make([]float64, len(src)/2+4)
		expected = make([]float64, len(dst))
		i16LEToF64(expected, src)
		I16LEToF64(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}
	})
}

func TestI16LEToF32(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		src := []byte{
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80,
			0x80, 0x00, 0x00, 0xff,
		}
		dst := make([]float32, len(src)/2)
		expected := make([]float32, len(dst))
		i16LEToF32(expected, src)
		I16LEToF32(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}
		i16LEToF32(expected, src)
		I16LEToF32(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}

		// Unaligned
		src = src[1 : len(src)-1]
		dst = make([]float32, len(src)/2+1)[1:]
		expected = make([]float32, len(dst))
		i16LEToF32(expected, src)
		I16LEToF32(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}

		// Make sure there's non-zero value after the expected length of the slice
		// to detect out of bound access.
		src = []byte{0x00, 0x00, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x80, 0x64, 0x00, 0xff, 0x00}[:10]
		dst = make([]float32, len(src)/2+4)
		expected = make([]float32, len(dst))
		i16LEToF32(expected, src)
		I16LEToF32(dst, src)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
			}
		}
	})
}

func BenchmarkU8ToI16(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]int16, len(src))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		U8ToI16(dst, src)
	}
}

func BenchmarkU8ToI16_Go(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]int16, len(src))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		u8ToI16(dst, src)
	}
}

func BenchmarkU8ToI16LE(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]byte, len(src)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		U8ToI16LE(dst, src)
	}
}

func BenchmarkU8ToI16LE_Unaligned(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]byte, len(src)*2+3)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		U8ToI16LE(dst, src)
	}
}

func BenchmarkU8ToI16LE_Go(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]byte, len(src)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		u8ToI16LE(dst, src)
	}
}

// A size sweep like the math32 ones. sz.n is the sample count, which is also
// the input byte count; the output is four times that.
func BenchmarkU8ToF32(b *testing.B) {
	for _, sz := range simdBenchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]byte, sz.n)
			dst := make([]float32, sz.n)
			for i := range src {
				src[i] = byte(i * 7)
			}
			b.SetBytes(int64(sz.n))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				U8ToF32(dst, src)
			}
		})
	}
}

func BenchmarkU8ToF32_Unaligned(b *testing.B) {
	src := make([]byte, benchSize+1)[1:]
	dst := make([]float32, len(src)+1)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		U8ToF32(dst, src)
	}
}

func BenchmarkI8ToF32(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]float32, len(src))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		I8ToF32(dst, src)
	}
}

func BenchmarkI8ToF32_Unaligned(b *testing.B) {
	src := make([]byte, benchSize+1)[1:]
	dst := make([]float32, len(src)+1)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		I8ToF32(dst, src)
	}
}

func BenchmarkI8ToF32_Go(b *testing.B) {
	src := make([]byte, benchSize)
	dst := make([]float32, len(src))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		i8ToF32(dst, src)
	}
}

// A size sweep like the math32 ones. sz.n is the sample count: the input is
// 2*sz.n bytes and the output sz.n complex64, and SetBytes reports the input.
func BenchmarkU8ToC64(b *testing.B) {
	for _, sz := range simdBenchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]byte, sz.n*2)
			dst := make([]complex64, sz.n)
			for i := range src {
				src[i] = byte(i * 7)
			}
			b.SetBytes(int64(sz.n) * 2)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				U8ToC64(dst, src)
			}
		})
	}
}

func BenchmarkU8ToC64_Unaligned(b *testing.B) {
	src := make([]byte, benchSize+1)[1:]
	dst := make([]complex64, len(src)/2+1)[1:]
	b.SetBytes(benchSize / 2)
	b.ResetTimer()
	for range b.N {
		U8ToC64(dst, src)
	}
}

func BenchmarkF32ToI16(b *testing.B) {
	src := make([]float32, benchSize)
	dst := make([]int16, len(src))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		F32ToI16(dst, src)
	}
}

func BenchmarkF32ToI16_Unaligned(b *testing.B) {
	src := make([]float32, benchSize+1)[1:]
	dst := make([]int16, len(src)+1)[1:]
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		F32ToI16(dst, src)
	}
}

func BenchmarkF32ToI16_Go(b *testing.B) {
	src := make([]float32, benchSize)
	dst := make([]int16, len(src))
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		f32ToI16(dst, src)
	}
}

func BenchmarkF32ToI16LE(b *testing.B) {
	src := make([]float32, benchSize)
	dst := make([]byte, len(src)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		F32ToI16LE(dst, src)
	}
}

func BenchmarkF32ToI16LE_Go(b *testing.B) {
	src := make([]float32, benchSize)
	dst := make([]byte, len(src)*2)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		f32ToI16LE(dst, src)
	}
}

func BenchmarkI16LEToF64(b *testing.B) {
	src := make([]byte, benchSize*2)
	dst := make([]float64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		I16LEToF64(dst, src)
	}
}

func BenchmarkI16LEToF64_Go(b *testing.B) {
	src := make([]byte, benchSize*2)
	dst := make([]float64, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		i16LEToF64(dst, src)
	}
}

func BenchmarkI16LEToF32(b *testing.B) {
	src := make([]byte, benchSize*2)
	dst := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		I16LEToF32(dst, src)
	}
}

func BenchmarkI16LEToF32_Go(b *testing.B) {
	src := make([]byte, benchSize*2)
	dst := make([]float32, benchSize)
	b.SetBytes(benchSize)
	b.ResetTimer()
	for range b.N {
		i16LEToF32(dst, src)
	}
}

func TestI16ToI16LE(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		src := []int16{0, 1, -1, 0x1234, -0x1234, 32767, -32768, 0x0100}
		for n := range len(src) + 1 {
			in := src[:n]
			// Two extra bytes to catch a write past the end.
			dst := make([]byte, n*2+2)
			expected := make([]byte, n*2+2)
			i16ToI16LE(expected, in)
			I16ToI16LE(dst, in)
			if !bytes.Equal(dst, expected) {
				t.Fatalf("n=%d:\n%+v\n%+v", n, dst, expected)
			}
		}

		// A short output truncates rather than overruns.
		dst := make([]byte, 5)
		I16ToI16LE(dst, src)
		expected := make([]byte, 5)
		i16ToI16LE(expected, src)
		if !bytes.Equal(dst, expected) {
			t.Fatalf("short dst:\n%+v\n%+v", dst, expected)
		}
	})
}

// TestF32ToI16Saturation covers the samples past full scale. int16(v) for an
// out-of-range v is implementation-defined in Go, so it used to fold an
// overdriven sample to the opposite rail -- 40000 came out as -25536 on arm64 --
// and it did so differently per architecture.
func TestF32ToI16Saturation(t *testing.T) {
	src := []float32{
		0, 1, -1, 32766.4, 32767.6, -32767.6, -32769, 40000, -40000,
		1e10, -1e10, 100000, -100000,
		float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)),
	}
	want := []int16{
		0, 1, -1, 32766, 32767, -32767, -32768, 32767, -32768,
		32767, -32768, 32767, -32768,
		0, 32767, -32768,
	}

	got := make([]int16, len(src))
	f32ToI16(got, src)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("f32ToI16(%v) = %d, want %d", src[i], got[i], want[i])
		}
	}

	// Every length, so the assembly's vector body, its one-vector loop and its
	// scalar tail all see out-of-range values.
	simdTest(t, func(t *testing.T) {
		for n := range len(src) + 1 {
			in := src[:n]
			asm := make([]int16, n)
			ref := make([]int16, n)
			F32ToI16(asm, in)
			f32ToI16(ref, in)
			for i := range ref {
				if asm[i] != ref[i] {
					t.Fatalf("n=%d i=%d in=%v: asm %d, go %d", n, i, in[i], asm[i], ref[i])
				}
			}

			bAsm := make([]byte, n*2)
			bRef := make([]byte, n*2)
			F32ToI16LE(bAsm, in)
			f32ToI16LE(bRef, in)
			if !bytes.Equal(bAsm, bRef) {
				t.Fatalf("ble n=%d:\n%+v\n%+v", n, bAsm, bRef)
			}
		}
	})
}

// TestConversionsEmpty checks every conversion against an empty input. I8ToF32
// carried a stale bounds-check hint that indexed dst[-1], and three of the
// four architectures reach that reference through a tail jump.
func TestConversionsEmpty(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		U8ToI16(nil, nil)
		U8ToI16LE(nil, nil)
		U8ToF32(nil, nil)
		U8ToC64(nil, nil)
		I8ToF32(nil, nil)
		I8ToC64(nil, nil)
		C64ToI8(nil, nil)
		F32ToI16(nil, nil)
		F32ToI16LE(nil, nil)
		I16ToI16LE(nil, nil)
		I16LEToF64(nil, nil)
		I16LEToF32(nil, nil)
		I32LEToF32(nil, nil)
		F32ToF32LE(nil, nil)

		// A non-empty input with an empty output has to write nothing too.
		I8ToF32(nil, []byte{1, 2, 3})
		U8ToF32(nil, []byte{1, 2, 3})
		F32ToI16(nil, []float32{1, 2, 3})
		I16ToI16LE(nil, []int16{1, 2, 3})
	})
}

func TestI8ToC64(t *testing.T) {
	src := []byte{0, 1, 0xff, 127, 0x80, 3}
	for n := range len(src) + 1 {
		in := src[:n]
		dst := make([]complex64, n/2+1)
		I8ToC64(dst, in)
		for i := range n / 2 {
			want := complex(float32(int8(in[i*2])), float32(int8(in[i*2+1])))
			if dst[i] != want {
				t.Fatalf("n=%d [%d] = %v, want %v", n, i, dst[i], want)
			}
		}
		if dst[n/2] != 0 {
			t.Fatalf("n=%d wrote past the end: %v", n, dst[n/2])
		}
	}
}

func TestC64ToI8(t *testing.T) {
	src := []complex64{complex(0, 1), complex(-1, 127), complex(-128, 3)}
	dst := make([]byte, len(src)*2+1)
	C64ToI8(dst, src)
	want := []byte{0, 1, 0xff, 127, 0x80, 3, 0}
	for i, v := range want {
		if dst[i] != v {
			t.Fatalf("[%d] = %d, want %d (%v)", i, dst[i], v, dst)
		}
	}

	// A short output stops on a whole complex value, not half of one.
	short := make([]byte, 3)
	C64ToI8(short, src)
	if short[2] != 0 {
		t.Fatalf("wrote a partial complex value: %v", short)
	}
}

func TestI32LEToF32(t *testing.T) {
	src := []byte{
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0x7f,
		0x00, 0x00, 0x00, 0x80,
		0x78, 0x56, 0x34, 0x12,
	}
	want := []float32{0, 1, -1, math.MaxInt32, math.MinInt32, 0x12345678}
	dst := make([]float32, len(want)+1)
	I32LEToF32(dst, src)
	for i, v := range want {
		if dst[i] != v {
			t.Fatalf("[%d] = %v, want %v", i, dst[i], v)
		}
	}
	if dst[len(want)] != 0 {
		t.Fatalf("wrote past the end: %v", dst)
	}

	// A trailing partial int32 is ignored.
	partial := make([]float32, len(want))
	I32LEToF32(partial, src[:len(src)-2])
	for i := range len(want) - 1 {
		if partial[i] != want[i] {
			t.Fatalf("[%d] = %v, want %v", i, partial[i], want[i])
		}
	}
	if partial[len(want)-1] != 0 {
		t.Fatalf("converted a partial int32: %v", partial)
	}
}

func TestF32ToF32LE(t *testing.T) {
	src := []float32{0, 1, -1, 3.14159, float32(math.Inf(1))}
	dst := make([]byte, len(src)*4+3)
	F32ToF32LE(dst, src)
	for i, v := range src {
		got := math.Float32frombits(binary.LittleEndian.Uint32(dst[i*4:]))
		if got != v {
			t.Fatalf("[%d] = %v, want %v", i, got, v)
		}
	}
	for _, b := range dst[len(src)*4:] {
		if b != 0 {
			t.Fatalf("wrote past the end: %+v", dst)
		}
	}

	// A short output stops on a whole float32.
	short := make([]byte, 7)
	F32ToF32LE(short, src)
	if !bytes.Equal(short[4:], []byte{0, 0, 0}) {
		t.Fatalf("wrote a partial float32: %+v", short)
	}
}

// FuzzI16LEToF32 checks the assembly against the Go reference.
func FuzzI16LEToF32(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x80})
	f.Add([]byte{0xff, 0x7f, 0x00, 0x80, 0x01, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		dst := make([]float32, len(data)/2)
		expected := make([]float32, len(data)/2)
		I16LEToF32(dst, data)
		i16LEToF32(expected, data)
		for i, v := range expected {
			if dst[i] != v {
				t.Fatalf("[%d]: asm %v, go %v", i, dst[i], v)
			}
		}
	})
}
