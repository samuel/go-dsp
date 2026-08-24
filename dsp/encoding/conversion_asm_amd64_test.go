//go:build amd64

package encoding

import "testing"

// TestU8ToF32Asm exercises the generated assembly and the scalar fallback
// directly. simdTest only toggles the feature flags the assembly reads, and
// under Rosetta useSSE4 is set while HasAVX is not, so this is what covers the
// SSE2 half on a machine that reports SSE4.
func TestU8ToF32Asm(t *testing.T) {
	impls := []struct {
		name string
		fn   func(dst []float32, src []byte)
	}{
		{"asm", u8ToF32Asm},
		{"scalar", u8ToF32},
	}
	for _, im := range impls {
		t.Run(im.name, func(t *testing.T) {
			simdTest(t, func(t *testing.T) {
				im.fn(nil, nil)
				for n := range 200 {
					src := make([]byte, n)
					expected := make([]float32, n)
					for i := range src {
						src[i] = byte(i * 7)
						expected[i] = u8ToF32Ref(src[i])
					}

					dst := make([]float32, n)
					im.fn(dst, src)
					for i, v := range expected {
						if dst[i] != v {
							t.Fatalf("len %d: dst[%d] = %v, want %v", n, i, dst[i], v)
						}
					}

					ui := make([]byte, n+1)[1:]
					copy(ui, src)
					uo := make([]float32, n+1)[1:]
					im.fn(uo, ui)
					for i, v := range expected {
						if uo[i] != v {
							t.Fatalf("len %d unaligned: dst[%d] = %v, want %v", n, i, uo[i], v)
						}
					}
				}

				all := make([]byte, 256)
				for i := range all {
					all[i] = byte(i)
				}
				out := make([]float32, 256)
				im.fn(out, all)
				for i, v := range all {
					if want := u8ToF32Ref(v); out[i] != want {
						t.Fatalf("src %d: got %v want %v", v, out[i], want)
					}
				}
			})
		})
	}
}
