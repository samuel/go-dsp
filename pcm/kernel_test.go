package pcm

import (
	"bytes"
	"math"
	"math/rand/v2"
	"testing"
	"unsafe"
)

type decodeKernel[F float] struct {
	name   string
	width  int
	kernel func([]F, []byte)
	ref    func([]F, []byte)
}

type encodeKernel[F float] struct {
	name   string
	width  int
	kernel func([]byte, []F)
	ref    func([]byte, []F)
}

var decodeF32Kernels = []decodeKernel[float32]{
	{"u8ToF32", 1, u8ToF32, u8ToFloatRef[float32]},
	{"i8ToF32", 1, i8ToF32, i8ToFloatRef[float32]},
	{"i16LEToF32", 2, i16LEToF32, i16LEToFloatRef[float32]},
	{"i24LEToF32", 3, i24LEToF32, i24LEToFloatRef[float32]},
	{"i32LEToF32", 4, i32LEToF32, i32LEToFloatRef[float32]},
	{"f32LEToF32", 4, f32LEToF32, f32LEToFloatRef[float32]},
	{"f64LEToF32", 8, f64LEToF32, f64LEToFloatRef[float32]},
	{"alawToF32", 1, alawToF32, alawToFloatRef[float32]},
	{"mulawToF32", 1, mulawToF32, mulawToFloatRef[float32]},
}

var decodeF64Kernels = []decodeKernel[float64]{
	{"u8ToF64", 1, u8ToF64, u8ToFloatRef[float64]},
	{"i8ToF64", 1, i8ToF64, i8ToFloatRef[float64]},
	{"i16LEToF64", 2, i16LEToF64, i16LEToFloatRef[float64]},
	{"i24LEToF64", 3, i24LEToF64, i24LEToFloatRef[float64]},
	{"i32LEToF64", 4, i32LEToF64, i32LEToFloatRef[float64]},
	{"f32LEToF64", 4, f32LEToF64, f32LEToFloatRef[float64]},
	{"f64LEToF64", 8, f64LEToF64, f64LEToFloatRef[float64]},
	{"alawToF64", 1, alawToF64, alawToFloatRef[float64]},
	{"mulawToF64", 1, mulawToF64, mulawToFloatRef[float64]},
}

var encodeF32Kernels = []encodeKernel[float32]{
	{"f32ToU8", 1, f32ToU8, floatToU8Ref[float32]},
	{"f32ToI8", 1, f32ToI8, floatToI8Ref[float32]},
	{"f32ToI16LE", 2, f32ToI16LE, floatToI16LERef[float32]},
	{"f32ToI24LE", 3, f32ToI24LE, floatToI24LERef[float32]},
	{"f32ToI32LE", 4, f32ToI32LE, floatToI32LERef[float32]},
	{"f32ToF32LE", 4, f32ToF32LE, floatToF32LERef[float32]},
	{"f32ToF64LE", 8, f32ToF64LE, floatToF64LERef[float32]},
	{"f32ToALaw", 1, f32ToALaw, floatToALawRef[float32]},
	{"f32ToMuLaw", 1, f32ToMuLaw, floatToMuLawRef[float32]},
}

var encodeF64Kernels = []encodeKernel[float64]{
	{"f64ToU8", 1, f64ToU8, floatToU8Ref[float64]},
	{"f64ToI8", 1, f64ToI8, floatToI8Ref[float64]},
	{"f64ToI16LE", 2, f64ToI16LE, floatToI16LERef[float64]},
	{"f64ToI24LE", 3, f64ToI24LE, floatToI24LERef[float64]},
	{"f64ToI32LE", 4, f64ToI32LE, floatToI32LERef[float64]},
	{"f64ToF32LE", 4, f64ToF32LE, floatToF32LERef[float64]},
	{"f64ToF64LE", 8, f64ToF64LE, floatToF64LERef[float64]},
	{"f64ToALaw", 1, f64ToALaw, floatToALawRef[float64]},
	{"f64ToMuLaw", 1, f64ToMuLaw, floatToMuLawRef[float64]},
}

// TestKernelsMatchReference runs every kernel against its reference on every
// CPU path this machine can take: every length from 0 to 200, a dst and a src
// at every offset within a 16-byte line, and both slices longer than the
// other. The arena around each slice is poisoned, so a write out of bounds
// fails as surely as a wrong value.
func TestKernelsMatchReference(t *testing.T) {
	simdTest(t, func(t *testing.T) {
		for _, k := range decodeF32Kernels {
			t.Run(k.name, func(t *testing.T) { checkDecodeKernel(t, k) })
		}
		for _, k := range decodeF64Kernels {
			t.Run(k.name, func(t *testing.T) { checkDecodeKernel(t, k) })
		}
		for _, k := range encodeF32Kernels {
			t.Run(k.name, func(t *testing.T) { checkEncodeKernel(t, k) })
		}
		for _, k := range encodeF64Kernels {
			t.Run(k.name, func(t *testing.T) { checkEncodeKernel(t, k) })
		}
	})
}

// kernelLengths are the sample counts each kernel is run at: every one up to
// 200, which reaches each vector body, its remainder steps and the scalar
// tail, then a few past every block size.
func kernelLengths() []int {
	var ns []int
	for n := range 201 {
		ns = append(ns, n)
	}
	return append(ns, 255, 256, 257, 1023, 1024, 1025)
}

// offsets are the misalignments tried for dst and src, in elements past a
// 16-byte boundary.
var offsets = [][2]int{{0, 0}, {1, 0}, {0, 1}, {3, 5}, {7, 2}}

// extras lengthen or shorten dst and src by a sample, so that each bounds the
// count in turn.
var extras = [][2]int{{0, 0}, {-1, 0}, {1, 0}, {0, -1}, {0, 1}}

const poison = 0xa5

func checkDecodeKernel[F float](t *testing.T, k decodeKernel[F]) {
	t.Helper()
	rng := rand.New(rand.NewPCG(1, uint64(k.width)))
	for _, n := range kernelLengths() {
		src := make([]byte, n*k.width)
		fillBytes(rng, src)
		want := make([]F, n)
		k.ref(want, src)

		for _, off := range offsets {
			do, so := off[0], off[1]
			for _, extra := range extras {
				dn, sn := n+extra[0], n*k.width+extra[1]*k.width
				if dn < 0 || sn < 0 {
					continue
				}
				m := min(dn, sn/k.width)
				d, darena := guarded[F](dn, do)
				s, _ := guarded[byte](sn, so)
				copy(s, src)
				if sn > len(src) {
					fillBytes(rng, s[len(src):])
				}
				k.kernel(d, s)
				for i := range m {
					if !sameFloat(d[i], want[i]) {
						t.Fatalf("n %d dst+%d src+%d %v: dst[%d] = %v, want %v", n, do, so, extra, i, d[i], want[i])
					}
				}
				checkUntouched(t, d[m:], darena, d, n)
			}
		}
	}
}

func checkEncodeKernel[F float](t *testing.T, k encodeKernel[F]) {
	t.Helper()
	rng := rand.New(rand.NewPCG(2, uint64(k.width)))
	for _, n := range kernelLengths() {
		src := make([]F, n)
		fillEncode(rng, src, k.width)
		want := make([]byte, n*k.width)
		k.ref(want, src)

		for _, off := range offsets {
			do, so := off[0], off[1]
			for _, extra := range extras {
				dn, sn := n*k.width+extra[0]*k.width, n+extra[1]
				if dn < 0 || sn < 0 {
					continue
				}
				m := min(dn/k.width, sn)
				d, darena := guarded[byte](dn, do)
				s, _ := guarded[F](sn, so)
				copy(s, src)
				for i := n; i < sn; i++ {
					s[i] = 0.25
				}
				k.kernel(d, s)
				if got := d[:min(m, n)*k.width]; !bytes.Equal(got, want[:len(got)]) {
					for i := range got {
						if got[i] != want[i] {
							j := i / k.width
							t.Fatalf("n %d dst+%d src+%d %v: sample %d (%v) encodes to % x, want % x",
								n, do, so, extra, j, s[j], got[j*k.width:j*k.width+k.width], want[j*k.width:j*k.width+k.width])
						}
					}
				}
				checkUntouched(t, d[m*k.width:], darena, d, n)
			}
		}
	}
}

// guarded returns an n-element slice at offset elements past a 16-byte
// boundary, inside an arena whose every byte, the slice's included, starts as
// poison.
func guarded[E any](n, offset int) ([]E, []byte) {
	var zero E
	size := int(unsafe.Sizeof(zero))
	const pad = 64
	arena := make([]byte, 2*pad+(n+offset)*size+16)
	for i := range arena {
		arena[i] = poison
	}
	base := uintptr(unsafe.Pointer(&arena[0]))
	start := pad + int((16-base%16)%16) + offset*size
	s := unsafe.Slice((*E)(unsafe.Pointer(&arena[start])), n)
	return s, arena
}

// checkUntouched fails if any byte of the arena outside the elements the
// kernel wrote changed.
func checkUntouched[E any](t *testing.T, rest []E, arena []byte, s []E, n int) {
	t.Helper()
	var zero E
	size := int(unsafe.Sizeof(zero))
	base := uintptr(unsafe.Pointer(&arena[0]))
	written := len(s) - len(rest)
	lo := 0
	if len(s) > 0 {
		lo = int(uintptr(unsafe.Pointer(unsafe.SliceData(s))) - base)
	}
	hi := lo + written*size
	for i, b := range arena {
		if (i < lo || i >= hi) && b != poison {
			t.Fatalf("n %d: arena byte %d changed (written range [%d, %d))", n, i, lo, hi)
		}
	}
}

func sameFloat[F float](a, b F) bool {
	if a != a && b != b { //nolint:gocritic // NaN check
		return true
	}
	return a == b && math.Signbit(float64(a)) == math.Signbit(float64(b))
}

// fillBytes fills b with random bytes, salted with the values on either side
// of every sign boundary.
func fillBytes(rng *rand.Rand, b []byte) {
	for i := range b {
		switch rng.IntN(8) {
		case 0:
			b[i] = 0x80
		case 1:
			b[i] = 0x7f
		case 2:
			b[i] = 0xff
		case 3:
			b[i] = 0
		default:
			b[i] = byte(rng.Uint32())
		}
	}
}

// fillEncode fills src with samples for a format width bytes wide: ordinary
// values, exact ties between codes, the rails and just past them, and the
// non-finite values.
func fillEncode[F float](rng *rand.Rand, src []F, width int) {
	bits := min(width*8, 32)
	step := math.Ldexp(1, 1-bits)
	for i := range src {
		var v float64
		switch rng.IntN(12) {
		case 0:
			v = (float64(rng.IntN(1<<min(bits, 20))-(1<<min(bits, 20)/2)) + 0.5) * step
		case 1:
			v = 1
		case 2:
			v = -1
		case 3:
			v = 1 + step
		case 4:
			v = -1 - step
		case 5:
			v = math.NaN()
		case 6:
			v = math.Inf(1 - 2*rng.IntN(2))
		case 7:
			v = math.Copysign(0, -1)
		case 8:
			v = (rng.Float64()*2 - 1) * 1e6
		case 9:
			v = 1 - step/2
		default:
			v = rng.Float64()*2.2 - 1.1
		}
		src[i] = F(v)
	}
}
