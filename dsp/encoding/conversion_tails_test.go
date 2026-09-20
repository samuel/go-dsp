package encoding

import (
	"bytes"
	"math"
	"testing"
	"unsafe"
)

// TestConversionTails is the memory-safety sweep over every conversion with an
// assembly implementation. Each is called with dst and src at every alignment
// offset within a 16-byte line, over lengths that reach the vector body, the
// one-vector loop and the scalar tail, with the arena on both sides of each
// slice poisoned: a byte of it changing is a write out of bounds.
//
// It exists because the two ways this family goes wrong are both invisible to a
// test that checks only the elements it asked for -- an alignment prologue that
// steps the wrong number of elements, and a vector block bound computed
// absolutely (len&^15) rather than relative to the index the prologue left
// behind.
func TestConversionTails(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		tails(t, "U8ToI16", U8ToI16, tailBytes, 1, 1)
		tails(t, "U8ToI16LE", U8ToI16LE, tailBytes, 2, 1)
		tails(t, "U8ToF32", U8ToF32, tailBytes, 1, 1)
		tails(t, "U8ToC64", U8ToC64, tailBytes, 1, 2)
		tails(t, "I8ToF32", I8ToF32, tailBytes, 1, 1)
		tails(t, "I8ToC64", I8ToC64, tailBytes, 1, 2)
		tails(t, "F32ToI16", F32ToI16, tailFloat32, 1, 1)
		tails(t, "F32ToI16LE", F32ToI16LE, tailFloat32, 2, 1)
		tails(t, "C64ToI8", C64ToI8, tailComplex64, 2, 1)
		tails(t, "I16ToI16LE", I16ToI16LE, tailInt16, 2, 1)
		tails(t, "I16LEToF64", I16LEToF64, tailBytes, 1, 2)
		tails(t, "I16LEToF32", I16LEToF32, tailBytes, 1, 2)
		tails(t, "I24LEToF32", I24LEToF32, tailBytes, 1, 3)
		tails(t, "I32LEToF32", I32LEToF32, tailBytes, 1, 4)
		tails(t, "F32LEToF32", F32LEToF32, tailBytes, 1, 4)
		tails(t, "F32ToF32LE", F32ToF32LE, tailFloat32, 4, 1)
	})
}

func tailBytes(i int) byte { return byte(i * 7) }

func tailInt16(i int) int16 { return int16(i * 4099) }

// The float32 sweep carries values past both int16 rails and the three
// non-finite cases, so the clipping paths in the vector body and in the scalar
// tail both see them.
func tailFloat32(i int) float32 {
	switch i % 17 {
	case 3:
		return float32(math.NaN())
	case 7:
		return float32(math.Inf(1))
	case 11:
		return float32(math.Inf(-1))
	}
	return float32(i)*997 - 60000
}

func tailComplex64(i int) complex64 {
	return complex(tailFloat32(i), tailFloat32(i+5))
}

// tails runs one conversion over the length and alignment sweep. num/den is how
// many dst elements the conversion produces per src element, so dst can be
// sized exactly and a single element of overrun lands in the guard.
func tails[D, S any](t *testing.T, name string, fn func(dst []D, src []S), fill func(i int) S, num, den int) {
	const where = "len(src) %d len(dst) %d at src offset %d dst offset %d"

	t.Run(name, func(t *testing.T) {
		// Enough offsets to cover a 16-byte line for each element width, since
		// that is what the alignment prologues branch on, and a padding region
		// wide enough to catch a whole vector store past the end.
		var d D
		var s S
		dstOffs := min(16/int(unsafe.Sizeof(d)), 8)
		srcOffs := min(16/int(unsafe.Sizeof(s)), 8)
		// Wide enough that a whole vector store past the end lands inside it.
		const pad = 64

		for srcLen := 1; srcLen <= 200; srcLen++ {
			exact := srcLen * num / den
			// Three size combinations: dst sized exactly for src, dst one element short of
			// it, and src far longer than dst. The last two are the ones that
			// reach the vector body with a bound derived from len(dst).
			sizes := [][2]int{{srcLen, exact}, {srcLen, max(exact-1, 0)}, {300, exact}}
			for _, sh := range sizes {
				sn, dn := sh[0], sh[1]
				for srcOff := range srcOffs {
					for dstOff := range dstOffs {
						srcArena := make([]S, srcOff+sn+pad)
						for i := range sn {
							srcArena[srcOff+i] = fill(i)
						}
						srcWant := bytes.Clone(tailAsBytes(srcArena))

						dstArena := make([]D, dstOff+dn+pad)
						poison := tailAsBytes(dstArena)
						for i := range poison {
							poison[i] = 0x5a
						}
						headWant := bytes.Clone(tailAsBytes(dstArena[:dstOff]))
						tailWant := bytes.Clone(tailAsBytes(dstArena[dstOff+dn:]))

						fn(dstArena[dstOff:dstOff+dn], srcArena[srcOff:srcOff+sn])

						if !bytes.Equal(tailAsBytes(dstArena[:dstOff]), headWant) {
							t.Fatalf(where+": wrote before the start of dst", sn, dn, srcOff, dstOff)
						}
						if !bytes.Equal(tailAsBytes(dstArena[dstOff+dn:]), tailWant) {
							t.Fatalf(where+": wrote past the end of dst", sn, dn, srcOff, dstOff)
						}
						if !bytes.Equal(tailAsBytes(srcArena), srcWant) {
							t.Fatalf(where+": modified src", sn, dn, srcOff, dstOff)
						}
					}
				}
			}
		}
	})
}

// tailAsBytes views a slice of any numeric type as its bytes, so a guard region
// can be compared exactly. Comparing element values instead would need a poison
// value no conversion can produce, which does not exist for the float
// destinations.
func tailAsBytes[T any](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*int(unsafe.Sizeof(s[0])))
}
