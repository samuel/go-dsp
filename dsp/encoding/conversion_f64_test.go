package encoding

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestF64ConversionsKnownValues pins each conversion to literal codes. These
// have no unexported Go reference to check against because they are the
// reference, so the expectations are written out rather than computed the same
// way the code computes them.
func TestF64ConversionsKnownValues(t *testing.T) {
	t.Run("U8ToF64", func(t *testing.T) {
		// The 128 bias comes off, matching U8ToF32, so the range is -128 to 127.
		got := make([]float64, 5)
		U8ToF64(got, []byte{0x00, 0x80, 0xff, 0x81, 0x7f})
		want := []float64{-128, 0, 127, 1, -1}
		checkF64(t, got, want)
	})

	t.Run("I8ToF64", func(t *testing.T) {
		got := make([]float64, 5)
		I8ToF64(got, []byte{0x80, 0x00, 0x7f, 0x01, 0xff})
		checkF64(t, got, []float64{-128, 0, 127, 1, -1})
	})

	t.Run("I24LEToF64", func(t *testing.T) {
		got := make([]float64, 6)
		I24LEToF64(got, []byte{
			0x00, 0x00, 0x80, // -8388608, the bottom rail
			0x00, 0x00, 0x00, // 0
			0xff, 0xff, 0x7f, // 8388607, the top
			0xff, 0xff, 0xff, // -1
			0x01, 0x00, 0x00, // 1
			0x00, 0x00, 0xff, // -65536
		})
		checkF64(t, got, []float64{-8388608, 0, 8388607, -1, 1, -65536})
	})

	t.Run("I32LEToF64", func(t *testing.T) {
		got := make([]float64, 4)
		I32LEToF64(got, []byte{
			0x00, 0x00, 0x00, 0x80, // -2147483648
			0x00, 0x00, 0x00, 0x00, // 0
			0xff, 0xff, 0xff, 0x7f, // 2147483647
			0xff, 0xff, 0xff, 0xff, // -1
		})
		checkF64(t, got, []float64{-2147483648, 0, 2147483647, -1})
	})

	t.Run("F32LEToF64", func(t *testing.T) {
		got := make([]float64, 3)
		F32LEToF64(got, []byte{
			0x00, 0x00, 0x80, 0xbf, // -1
			0x00, 0x00, 0x00, 0x3f, // 0.5
			0x00, 0x00, 0x00, 0x00, // 0
		})
		checkF64(t, got, []float64{-1, 0.5, 0})
	})

	t.Run("F64LEToF64", func(t *testing.T) {
		got := make([]float64, 2)
		F64LEToF64(got, []byte{
			0, 0, 0, 0, 0, 0, 0xf0, 0xbf, // -1
			0, 0, 0, 0, 0, 0, 0xe0, 0x3f, // 0.5
		})
		checkF64(t, got, []float64{-1, 0.5})
	})
}

func checkF64(t *testing.T, got, want []float64) {
	t.Helper()
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestF64ConversionsAgreeWithF32 is the cross-check that matters: where the
// package already converts the same bytes to float32, the wider version has to
// agree, because the two are the same numbers.
func TestF64ConversionsAgreeWithF32(t *testing.T) {
	// Every byte value, so the 8-bit conversions are exhaustively covered.
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}

	t.Run("U8", func(t *testing.T) {
		wide := make([]float64, len(all))
		narrow := make([]float32, len(all))
		U8ToF64(wide, all)
		U8ToF32(narrow, all)
		for i := range all {
			if float32(wide[i]) != narrow[i] {
				t.Fatalf("byte %d: U8ToF64 gave %v, U8ToF32 gave %v", i, wide[i], narrow[i])
			}
		}
	})

	t.Run("I8", func(t *testing.T) {
		wide := make([]float64, len(all))
		narrow := make([]float32, len(all))
		I8ToF64(wide, all)
		I8ToF32(narrow, all)
		for i := range all {
			if float32(wide[i]) != narrow[i] {
				t.Fatalf("byte %d: I8ToF64 gave %v, I8ToF32 gave %v", i, wide[i], narrow[i])
			}
		}
	})

	t.Run("I16LE", func(t *testing.T) {
		// The assembly-backed float64 conversion is the one already in the
		// package; this only confirms the two widths still agree.
		src := make([]byte, 2*1000)
		for i := range src {
			src[i] = byte(i * 31)
		}
		wide := make([]float64, 1000)
		narrow := make([]float32, 1000)
		I16LEToF64(wide, src)
		I16LEToF32(narrow, src)
		for i := range wide {
			if float32(wide[i]) != narrow[i] {
				t.Fatalf("sample %d: %v against %v", i, wide[i], narrow[i])
			}
		}
	})

	t.Run("I32LE", func(t *testing.T) {
		// Here the two genuinely differ, and that is the point: a 32-bit integer
		// does not fit float32's 24-bit mantissa, so the narrow conversion
		// rounds. The wide one must be exact, and rounding it must reproduce the
		// narrow one.
		src := make([]byte, 4*1000)
		for i := range src {
			src[i] = byte(i * 17)
		}
		wide := make([]float64, 1000)
		narrow := make([]float32, 1000)
		I32LEToF64(wide, src)
		I32LEToF32(narrow, src)

		var sawRounding bool
		for i := range wide {
			exact := float64(int32(binary.LittleEndian.Uint32(src[i*4:])))
			if wide[i] != exact {
				t.Fatalf("sample %d: I32LEToF64 gave %v, want the exact %v", i, wide[i], exact)
			}
			if float32(wide[i]) != narrow[i] {
				t.Fatalf("sample %d: rounding %v did not give I32LEToF32's %v", i, wide[i], narrow[i])
			}
			if float64(narrow[i]) != exact {
				sawRounding = true
			}
		}
		if !sawRounding {
			t.Error("no sample lost precision in float32, so this test proved nothing")
		}
	})
}

// TestF32LEToF64RoundTrip goes through the package's own writer, so the two
// halves check each other.
func TestF32LEToF64RoundTrip(t *testing.T) {
	want := []float32{0, 1, -1, 0.5, -0.25, 1e-30, 1e30, float32(math.Pi)}
	raw := make([]byte, 4*len(want))
	F32ToF32LE(raw, want)

	got := make([]float64, len(want))
	F32LEToF64(got, raw)
	for i := range want {
		if got[i] != float64(want[i]) {
			t.Errorf("sample %d = %v, want %v", i, got[i], float64(want[i]))
		}
	}
}

func TestF64LEToF64RoundTrip(t *testing.T) {
	want := []float64{0, 1, -1, 0.5, math.Pi, math.SmallestNonzeroFloat64, math.MaxFloat64}
	raw := make([]byte, 8*len(want))
	for i, v := range want {
		binary.LittleEndian.PutUint64(raw[i*8:], math.Float64bits(v))
	}
	got := make([]float64, len(want))
	F64LEToF64(got, raw)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestF64ConversionsMismatchedLengths is the package-wide contract: convert the
// smaller of what was given and leave the rest alone.
func TestF64ConversionsMismatchedLengths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width int
		fn    func([]float64, []byte)
	}{
		{"U8ToF64", 1, U8ToF64},
		{"I8ToF64", 1, I8ToF64},
		{"I16LEToF64", 2, I16LEToF64},
		{"I24LEToF64", 3, I24LEToF64},
		{"I32LEToF64", 4, I32LEToF64},
		{"F32LEToF64", 4, F32LEToF64},
		{"F64LEToF64", 8, F64LEToF64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range []int{0, 1, 2, 3, 7, 8, 9, 33} {
				// More output than input: the tail must not be written.
				src := make([]byte, n*tc.width)
				for i := range src {
					src[i] = byte(i + 1)
				}
				dst := make([]float64, n+4)
				const sentinel = 12345.0
				for i := range dst {
					dst[i] = sentinel
				}
				tc.fn(dst, src)
				for i := n; i < len(dst); i++ {
					if dst[i] != sentinel {
						t.Fatalf("n=%d: wrote past the src at %d", n, i)
					}
				}

				// More input than output: nothing may be written past dst.
				big := make([]byte, (n+4)*tc.width)
				small := make([]float64, n)
				tc.fn(small, big) // must not panic

				// A partial trailing sample is not a sample.
				if tc.width > 1 && n > 0 {
					short := make([]byte, n*tc.width-1)
					out := make([]float64, n)
					for i := range out {
						out[i] = sentinel
					}
					tc.fn(out, short)
					if out[n-1] != sentinel {
						t.Fatalf("n=%d: decoded a partial trailing sample", n)
					}
				}
			}
		})
	}
}
