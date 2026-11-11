//go:build goexperiment.simd

package f32

import (
	"simd/archsimd"
	"unsafe"

	"samuelks.com/dsp/internal/view"
)

// The archsimd FIR and dot-product kernels. fir.go defines the results; see
// its header for why vectorizing FIR across outputs and Dot over eight lanes
// reproduces the references bit for bit. Every multiply here is a separate
// Mul, never MulAdd: a fused multiply-add would skip the product's rounding.
//
// A strided FIR runs the Go reference, since its outputs do not sit in
// contiguous work.
//
// The loops follow math32_simd_amd64.go (array-pointer loads and stores,
// advancing slices) except inside the FIR tap loops, which walk work backwards
// and so cannot advance a slice. Slicing at a variable offset there costs a
// pair of LEAQs per load, and with eight accumulators live the addresses
// spill: 55 instructions per tap of the 256-bit block against 28 for loads at
// constant offsets from a pointer stepped back one sample per tap. So each
// block bounds-checks its whole window once, which keeps a short work a panic,
// and the tap loop steps an unsafe.Pointer inside it, from one sample past
// tap 0's position down to the window's start.

// ld4, ld8 and ld16 load the vector i floats past p.
func ld4(p unsafe.Pointer, i int) archsimd.Float32x4 {
	return archsimd.LoadFloat32x4Array((*[4]float32)(unsafe.Add(p, 4*i)))
}

func ld8(p unsafe.Pointer, i int) archsimd.Float32x8 {
	return archsimd.LoadFloat32x8Array((*[8]float32)(unsafe.Add(p, 4*i)))
}

func ld16(p unsafe.Pointer, i int) archsimd.Float32x16 {
	return archsimd.LoadFloat32x16Array((*[16]float32)(unsafe.Add(p, 4*i)))
}

func firF32(dst, work, taps []float32, stride int) {
	switch {
	case !haveSIMD():
		// Pre-AVX CPU; see Scale.
		firF32Asm(dst, work, taps, stride)
	case stride != 1:
		firF32Go(dst, work, taps, stride)
	default:
		k := firVec(dst, work, taps, 1)
		firF32Go(dst[k:], work[k:], taps, 1)
	}
}

func cfirReal(dst, work []complex64, taps []float32, stride int) {
	switch {
	case !haveSIMD():
		cfirRealAsm(dst, work, taps, stride)
	case stride != 1:
		cfirRealGo(dst, work, taps, stride)
	default:
		// The real filter over the interleaved parts, with each tap stepping
		// one complex sample, two floats.
		k := firVec(view.C64ToF32(dst), view.C64ToF32(work), taps, 2) / 2
		cfirRealGo(dst[k:], work[k:], taps, 1)
	}
}

func cfir(dst, work, taps []complex64, stride int) {
	switch {
	case !haveSIMD():
		cfirAsm(dst, work, taps, stride)
	case stride != 1:
		cfirGo(dst, work, taps, stride)
	default:
		k := cfirVec(view.C64ToF32(dst), view.C64ToF32(work), taps) / 2
		cfirGo(dst[k:], work[k:], taps, 1)
	}
}

func dotF32(x, c []float32) float32 {
	if !haveSIMD() {
		return dotF32Asm(x, c)
	}
	var s float32
	// Each lane is one serial chain across the whole input, so a wider vector
	// cannot shorten it without changing the result, and the 512-bit path
	// would change the tree. Eight lanes cover all eight accumulators.
	if simdWidth >= 256 {
		s = dotF32x8(x, c)
	} else {
		s = dotF32x4(x, c)
	}
	archsimd.ClearAVXUpperBits() // see Scale
	return s
}

func cdotReal(x []complex64, c []float32) complex64 {
	if !haveSIMD() {
		return cdotRealAsm(x, c)
	}
	s := cdotRealX4(view.C64ToF32(x), c)
	archsimd.ClearAVXUpperBits()
	return s
}

// firVec runs FIR over as many leading outputs of dst as whole vectors cover
// and returns how many it wrote, a multiple of four. Output f is the sum over
// taps of taps[j]*work[f+step*(len(taps)-1-j)]: step 1 is the real filter,
// step 2 the real filter over interleaved complex parts.
func firVec(dst, work, taps []float32, step int) int {
	var k int
	switch {
	case simdWidth >= 512:
		k = firX16(dst, work, taps, step)
	case simdWidth >= 256:
		k = firX8(dst, work, taps, step)
	default:
		k = firX4(dst, work, taps, step)
	}
	archsimd.ClearAVXUpperBits() // see Scale
	return k
}

// The FIR loops run eight vectors of outputs at once, one accumulator each,
// enough independent add chains to cover VADDPS's latency on two ports; each
// broadcast tap is shared by all eight. Then one step of four and single
// vectors, and the narrower width finishes what is left. p points at tap j's
// samples for the block's first output.

func firX16(dst, work, taps []float32, step int) int {
	n := len(dst)
	last := step * (len(taps) - 1)
	back := 4 * step
	for len(dst) >= 128 {
		p := unsafe.Pointer(&work[:last+128][last+step])
		var a0, a1, a2, a3, a4, a5, a6, a7 archsimd.Float32x16
		for _, c := range taps {
			h := archsimd.BroadcastFloat32x16(c)
			p = unsafe.Add(p, -back)
			a0 = a0.Add(ld16(p, 0).Mul(h))
			a1 = a1.Add(ld16(p, 16).Mul(h))
			a2 = a2.Add(ld16(p, 32).Mul(h))
			a3 = a3.Add(ld16(p, 48).Mul(h))
			a4 = a4.Add(ld16(p, 64).Mul(h))
			a5 = a5.Add(ld16(p, 80).Mul(h))
			a6 = a6.Add(ld16(p, 96).Mul(h))
			a7 = a7.Add(ld16(p, 112).Mul(h))
		}
		out := (*[128]float32)(dst[0:128])
		a0.StoreArray((*[16]float32)(out[0:16]))
		a1.StoreArray((*[16]float32)(out[16:32]))
		a2.StoreArray((*[16]float32)(out[32:48]))
		a3.StoreArray((*[16]float32)(out[48:64]))
		a4.StoreArray((*[16]float32)(out[64:80]))
		a5.StoreArray((*[16]float32)(out[80:96]))
		a6.StoreArray((*[16]float32)(out[96:112]))
		a7.StoreArray((*[16]float32)(out[112:128]))
		dst, work = dst[128:], work[128:]
	}
	if len(dst) >= 64 {
		p := unsafe.Pointer(&work[:last+64][last+step])
		var a0, a1, a2, a3 archsimd.Float32x16
		for _, c := range taps {
			h := archsimd.BroadcastFloat32x16(c)
			p = unsafe.Add(p, -back)
			a0 = a0.Add(ld16(p, 0).Mul(h))
			a1 = a1.Add(ld16(p, 16).Mul(h))
			a2 = a2.Add(ld16(p, 32).Mul(h))
			a3 = a3.Add(ld16(p, 48).Mul(h))
		}
		out := (*[64]float32)(dst[0:64])
		a0.StoreArray((*[16]float32)(out[0:16]))
		a1.StoreArray((*[16]float32)(out[16:32]))
		a2.StoreArray((*[16]float32)(out[32:48]))
		a3.StoreArray((*[16]float32)(out[48:64]))
		dst, work = dst[64:], work[64:]
	}
	for len(dst) >= 16 {
		p := unsafe.Pointer(&work[:last+16][last+step])
		var a archsimd.Float32x16
		for _, c := range taps {
			p = unsafe.Add(p, -back)
			a = a.Add(ld16(p, 0).Mul(archsimd.BroadcastFloat32x16(c)))
		}
		a.StoreArray((*[16]float32)(dst[0:16]))
		dst, work = dst[16:], work[16:]
	}
	return n - len(dst) + firX8(dst, work, taps, step)
}

func firX8(dst, work, taps []float32, step int) int {
	n := len(dst)
	last := step * (len(taps) - 1)
	back := 4 * step
	for len(dst) >= 64 {
		p := unsafe.Pointer(&work[:last+64][last+step])
		var a0, a1, a2, a3, a4, a5, a6, a7 archsimd.Float32x8
		for _, c := range taps {
			h := archsimd.BroadcastFloat32x8(c)
			p = unsafe.Add(p, -back)
			a0 = a0.Add(ld8(p, 0).Mul(h))
			a1 = a1.Add(ld8(p, 8).Mul(h))
			a2 = a2.Add(ld8(p, 16).Mul(h))
			a3 = a3.Add(ld8(p, 24).Mul(h))
			a4 = a4.Add(ld8(p, 32).Mul(h))
			a5 = a5.Add(ld8(p, 40).Mul(h))
			a6 = a6.Add(ld8(p, 48).Mul(h))
			a7 = a7.Add(ld8(p, 56).Mul(h))
		}
		out := (*[64]float32)(dst[0:64])
		a0.StoreArray((*[8]float32)(out[0:8]))
		a1.StoreArray((*[8]float32)(out[8:16]))
		a2.StoreArray((*[8]float32)(out[16:24]))
		a3.StoreArray((*[8]float32)(out[24:32]))
		a4.StoreArray((*[8]float32)(out[32:40]))
		a5.StoreArray((*[8]float32)(out[40:48]))
		a6.StoreArray((*[8]float32)(out[48:56]))
		a7.StoreArray((*[8]float32)(out[56:64]))
		dst, work = dst[64:], work[64:]
	}
	if len(dst) >= 32 {
		p := unsafe.Pointer(&work[:last+32][last+step])
		var a0, a1, a2, a3 archsimd.Float32x8
		for _, c := range taps {
			h := archsimd.BroadcastFloat32x8(c)
			p = unsafe.Add(p, -back)
			a0 = a0.Add(ld8(p, 0).Mul(h))
			a1 = a1.Add(ld8(p, 8).Mul(h))
			a2 = a2.Add(ld8(p, 16).Mul(h))
			a3 = a3.Add(ld8(p, 24).Mul(h))
		}
		out := (*[32]float32)(dst[0:32])
		a0.StoreArray((*[8]float32)(out[0:8]))
		a1.StoreArray((*[8]float32)(out[8:16]))
		a2.StoreArray((*[8]float32)(out[16:24]))
		a3.StoreArray((*[8]float32)(out[24:32]))
		dst, work = dst[32:], work[32:]
	}
	for len(dst) >= 8 {
		p := unsafe.Pointer(&work[:last+8][last+step])
		var a archsimd.Float32x8
		for _, c := range taps {
			p = unsafe.Add(p, -back)
			a = a.Add(ld8(p, 0).Mul(archsimd.BroadcastFloat32x8(c)))
		}
		a.StoreArray((*[8]float32)(dst[0:8]))
		dst, work = dst[8:], work[8:]
	}
	return n - len(dst) + firX4(dst, work, taps, step)
}

func firX4(dst, work, taps []float32, step int) int {
	n := len(dst)
	last := step * (len(taps) - 1)
	back := 4 * step
	for len(dst) >= 32 {
		p := unsafe.Pointer(&work[:last+32][last+step])
		var a0, a1, a2, a3, a4, a5, a6, a7 archsimd.Float32x4
		for _, c := range taps {
			h := archsimd.BroadcastFloat32x4(c)
			p = unsafe.Add(p, -back)
			a0 = a0.Add(ld4(p, 0).Mul(h))
			a1 = a1.Add(ld4(p, 4).Mul(h))
			a2 = a2.Add(ld4(p, 8).Mul(h))
			a3 = a3.Add(ld4(p, 12).Mul(h))
			a4 = a4.Add(ld4(p, 16).Mul(h))
			a5 = a5.Add(ld4(p, 20).Mul(h))
			a6 = a6.Add(ld4(p, 24).Mul(h))
			a7 = a7.Add(ld4(p, 28).Mul(h))
		}
		out := (*[32]float32)(dst[0:32])
		a0.StoreArray((*[4]float32)(out[0:4]))
		a1.StoreArray((*[4]float32)(out[4:8]))
		a2.StoreArray((*[4]float32)(out[8:12]))
		a3.StoreArray((*[4]float32)(out[12:16]))
		a4.StoreArray((*[4]float32)(out[16:20]))
		a5.StoreArray((*[4]float32)(out[20:24]))
		a6.StoreArray((*[4]float32)(out[24:28]))
		a7.StoreArray((*[4]float32)(out[28:32]))
		dst, work = dst[32:], work[32:]
	}
	if len(dst) >= 16 {
		p := unsafe.Pointer(&work[:last+16][last+step])
		var a0, a1, a2, a3 archsimd.Float32x4
		for _, c := range taps {
			h := archsimd.BroadcastFloat32x4(c)
			p = unsafe.Add(p, -back)
			a0 = a0.Add(ld4(p, 0).Mul(h))
			a1 = a1.Add(ld4(p, 4).Mul(h))
			a2 = a2.Add(ld4(p, 8).Mul(h))
			a3 = a3.Add(ld4(p, 12).Mul(h))
		}
		out := (*[16]float32)(dst[0:16])
		a0.StoreArray((*[4]float32)(out[0:4]))
		a1.StoreArray((*[4]float32)(out[4:8]))
		a2.StoreArray((*[4]float32)(out[8:12]))
		a3.StoreArray((*[4]float32)(out[12:16]))
		dst, work = dst[16:], work[16:]
	}
	for len(dst) >= 4 {
		p := unsafe.Pointer(&work[:last+4][last+step])
		var a archsimd.Float32x4
		for _, c := range taps {
			p = unsafe.Add(p, -back)
			a = a.Add(ld4(p, 0).Mul(archsimd.BroadcastFloat32x4(c)))
		}
		a.StoreArray((*[4]float32)(dst[0:4]))
		dst, work = dst[4:], work[4:]
	}
	return n - len(dst)
}

// cfirSigns negates the real lanes of the cross term. Rounding is symmetric,
// so round(vi*-ci) is exactly -round(ci*vi), and adding it is exactly the
// reference's subtraction.
var cfirSigns = [16]float32{-1, 1, -1, 1, -1, 1, -1, 1, -1, 1, -1, 1, -1, 1, -1, 1}

// cfirVec is firVec for complex taps over the interleaved views of dst and
// work. Each vector of work v contributes v*re(c) plus its pair-swapped copy
// times (-im(c), im(c)), the two rounded products of each part added before
// the accumulator, as in cfirGo. The swap (VSHUFPS) holds the loops to one
// vector per cycle, so four accumulators already outrun it.
func cfirVec(dst, work []float32, taps []complex64) int {
	var k int
	switch {
	case simdWidth >= 512:
		k = cfirX16(dst, work, taps)
	case simdWidth >= 256:
		k = cfirX8(dst, work, taps)
	default:
		k = cfirX4(dst, work, taps)
	}
	archsimd.ClearAVXUpperBits() // see Scale
	return k
}

func cfirX16(dst, work []float32, taps []complex64) int {
	n := len(dst)
	last := 2 * (len(taps) - 1)
	signs := archsimd.LoadFloat32x16Array(&cfirSigns)
	for len(dst) >= 64 {
		p := unsafe.Pointer(&work[:last+64][last+2])
		var a0, a1, a2, a3 archsimd.Float32x16
		for _, c := range taps {
			hr := archsimd.BroadcastFloat32x16(real(c))
			hi := archsimd.BroadcastFloat32x16(imag(c)).Mul(signs)
			p = unsafe.Add(p, -8)
			v0, v1, v2, v3 := ld16(p, 0), ld16(p, 16), ld16(p, 32), ld16(p, 48)
			a0 = a0.Add(v0.Mul(hr).Add(v0.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v0).Mul(hi)))
			a1 = a1.Add(v1.Mul(hr).Add(v1.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v1).Mul(hi)))
			a2 = a2.Add(v2.Mul(hr).Add(v2.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v2).Mul(hi)))
			a3 = a3.Add(v3.Mul(hr).Add(v3.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v3).Mul(hi)))
		}
		out := (*[64]float32)(dst[0:64])
		a0.StoreArray((*[16]float32)(out[0:16]))
		a1.StoreArray((*[16]float32)(out[16:32]))
		a2.StoreArray((*[16]float32)(out[32:48]))
		a3.StoreArray((*[16]float32)(out[48:64]))
		dst, work = dst[64:], work[64:]
	}
	for len(dst) >= 16 {
		p := unsafe.Pointer(&work[:last+16][last+2])
		var a archsimd.Float32x16
		for _, c := range taps {
			hr := archsimd.BroadcastFloat32x16(real(c))
			hi := archsimd.BroadcastFloat32x16(imag(c)).Mul(signs)
			p = unsafe.Add(p, -8)
			v := ld16(p, 0)
			a = a.Add(v.Mul(hr).Add(v.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v).Mul(hi)))
		}
		a.StoreArray((*[16]float32)(dst[0:16]))
		dst, work = dst[16:], work[16:]
	}
	return n - len(dst) + cfirX8(dst, work, taps)
}

func cfirX8(dst, work []float32, taps []complex64) int {
	n := len(dst)
	last := 2 * (len(taps) - 1)
	signs := archsimd.LoadFloat32x8Array((*[8]float32)(cfirSigns[0:8]))
	for len(dst) >= 32 {
		p := unsafe.Pointer(&work[:last+32][last+2])
		var a0, a1, a2, a3 archsimd.Float32x8
		for _, c := range taps {
			hr := archsimd.BroadcastFloat32x8(real(c))
			hi := archsimd.BroadcastFloat32x8(imag(c)).Mul(signs)
			p = unsafe.Add(p, -8)
			v0, v1, v2, v3 := ld8(p, 0), ld8(p, 8), ld8(p, 16), ld8(p, 24)
			a0 = a0.Add(v0.Mul(hr).Add(v0.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v0).Mul(hi)))
			a1 = a1.Add(v1.Mul(hr).Add(v1.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v1).Mul(hi)))
			a2 = a2.Add(v2.Mul(hr).Add(v2.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v2).Mul(hi)))
			a3 = a3.Add(v3.Mul(hr).Add(v3.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v3).Mul(hi)))
		}
		out := (*[32]float32)(dst[0:32])
		a0.StoreArray((*[8]float32)(out[0:8]))
		a1.StoreArray((*[8]float32)(out[8:16]))
		a2.StoreArray((*[8]float32)(out[16:24]))
		a3.StoreArray((*[8]float32)(out[24:32]))
		dst, work = dst[32:], work[32:]
	}
	for len(dst) >= 8 {
		p := unsafe.Pointer(&work[:last+8][last+2])
		var a archsimd.Float32x8
		for _, c := range taps {
			hr := archsimd.BroadcastFloat32x8(real(c))
			hi := archsimd.BroadcastFloat32x8(imag(c)).Mul(signs)
			p = unsafe.Add(p, -8)
			v := ld8(p, 0)
			a = a.Add(v.Mul(hr).Add(v.ConcatPermuteScalarsGrouped(1, 0, 3, 2, v).Mul(hi)))
		}
		a.StoreArray((*[8]float32)(dst[0:8]))
		dst, work = dst[8:], work[8:]
	}
	return n - len(dst) + cfirX4(dst, work, taps)
}

func cfirX4(dst, work []float32, taps []complex64) int {
	n := len(dst)
	last := 2 * (len(taps) - 1)
	signs := archsimd.LoadFloat32x4Array((*[4]float32)(cfirSigns[0:4]))
	for len(dst) >= 16 {
		p := unsafe.Pointer(&work[:last+16][last+2])
		var a0, a1, a2, a3 archsimd.Float32x4
		for _, c := range taps {
			hr := archsimd.BroadcastFloat32x4(real(c))
			hi := archsimd.BroadcastFloat32x4(imag(c)).Mul(signs)
			p = unsafe.Add(p, -8)
			v0, v1, v2, v3 := ld4(p, 0), ld4(p, 4), ld4(p, 8), ld4(p, 12)
			a0 = a0.Add(v0.Mul(hr).Add(v0.ConcatPermuteScalars(1, 0, 3, 2, v0).Mul(hi)))
			a1 = a1.Add(v1.Mul(hr).Add(v1.ConcatPermuteScalars(1, 0, 3, 2, v1).Mul(hi)))
			a2 = a2.Add(v2.Mul(hr).Add(v2.ConcatPermuteScalars(1, 0, 3, 2, v2).Mul(hi)))
			a3 = a3.Add(v3.Mul(hr).Add(v3.ConcatPermuteScalars(1, 0, 3, 2, v3).Mul(hi)))
		}
		out := (*[16]float32)(dst[0:16])
		a0.StoreArray((*[4]float32)(out[0:4]))
		a1.StoreArray((*[4]float32)(out[4:8]))
		a2.StoreArray((*[4]float32)(out[8:12]))
		a3.StoreArray((*[4]float32)(out[12:16]))
		dst, work = dst[16:], work[16:]
	}
	for len(dst) >= 4 {
		p := unsafe.Pointer(&work[:last+4][last+2])
		var a archsimd.Float32x4
		for _, c := range taps {
			hr := archsimd.BroadcastFloat32x4(real(c))
			hi := archsimd.BroadcastFloat32x4(imag(c)).Mul(signs)
			p = unsafe.Add(p, -8)
			v := ld4(p, 0)
			a = a.Add(v.Mul(hr).Add(v.ConcatPermuteScalars(1, 0, 3, 2, v).Mul(hi)))
		}
		a.StoreArray((*[4]float32)(dst[0:4]))
		dst, work = dst[4:], work[4:]
	}
	return n - len(dst)
}

// combine4 is combine8 over the eight accumulators split as lo and hi, in
// registers.
func combine4(lo, hi archsimd.Float32x4) float32 {
	t := lo.Add(hi)
	t = t.Add(t.ConcatPermuteScalars(2, 3, 2, 3, t))
	return t.GetElem(0) + t.GetElem(1)
}

// dotF32x8 holds the eight accumulators in one vector. The blocks of an
// iteration are added in order, so the unroll only saves loop overhead.
func dotF32x8(x, c []float32) float32 {
	var a archsimd.Float32x8
	for len(c) >= 32 {
		in := (*[32]float32)(x[0:32])
		cf := (*[32]float32)(c[0:32])
		p0 := archsimd.LoadFloat32x8Array((*[8]float32)(in[0:8])).Mul(archsimd.LoadFloat32x8Array((*[8]float32)(cf[0:8])))
		p1 := archsimd.LoadFloat32x8Array((*[8]float32)(in[8:16])).Mul(archsimd.LoadFloat32x8Array((*[8]float32)(cf[8:16])))
		p2 := archsimd.LoadFloat32x8Array((*[8]float32)(in[16:24])).Mul(archsimd.LoadFloat32x8Array((*[8]float32)(cf[16:24])))
		p3 := archsimd.LoadFloat32x8Array((*[8]float32)(in[24:32])).Mul(archsimd.LoadFloat32x8Array((*[8]float32)(cf[24:32])))
		a = a.Add(p0).Add(p1).Add(p2).Add(p3)
		x, c = x[32:], c[32:]
	}
	for len(c) >= 8 {
		p := archsimd.LoadFloat32x8Array((*[8]float32)(x[0:8])).Mul(archsimd.LoadFloat32x8Array((*[8]float32)(c[0:8])))
		a = a.Add(p)
		x, c = x[8:], c[8:]
	}
	sum := combine4(a.GetLo(), a.GetHi())
	for i, v := range c {
		sum += float32(x[i] * v)
	}
	return sum
}

// dotF32x4 holds accumulators 0-3 in lo and 4-7 in hi.
func dotF32x4(x, c []float32) float32 {
	var lo, hi archsimd.Float32x4
	for len(c) >= 16 {
		in := (*[16]float32)(x[0:16])
		cf := (*[16]float32)(c[0:16])
		p0 := archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cf[0:4])))
		p1 := archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cf[4:8])))
		p2 := archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cf[8:12])))
		p3 := archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cf[12:16])))
		lo = lo.Add(p0).Add(p2)
		hi = hi.Add(p1).Add(p3)
		x, c = x[16:], c[16:]
	}
	for len(c) >= 8 {
		in := (*[8]float32)(x[0:8])
		cf := (*[8]float32)(c[0:8])
		lo = lo.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cf[0:4]))))
		hi = hi.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).Mul(archsimd.LoadFloat32x4Array((*[4]float32)(cf[4:8]))))
		x, c = x[8:], c[8:]
	}
	sum := combine4(lo, hi)
	for i, v := range c {
		sum += float32(x[i] * v)
	}
	return sum
}

// cdotRealX4 is CDotReal over the interleaved view of x. The sixteen
// accumulators, eight per part, sit in four vectors as (re, im) pairs: a0
// holds complex lanes 0-1 of each block, a1 lanes 2-3, a2 lanes 4-5 and a3
// lanes 6-7, against coefficients duplicated into pairs. Each lane is a single
// serial chain whatever the vector width, so 128 bits serve every width.
func cdotRealX4(x, c []float32) complex64 {
	var a0, a1, a2, a3 archsimd.Float32x4
	for len(c) >= 8 {
		in := (*[16]float32)(x[0:16])
		cf := (*[8]float32)(c[0:8])
		cl := archsimd.LoadFloat32x4Array((*[4]float32)(cf[0:4]))
		ch := archsimd.LoadFloat32x4Array((*[4]float32)(cf[4:8]))
		a0 = a0.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[0:4])).Mul(cl.ConcatPermuteScalars(0, 0, 1, 1, cl)))
		a1 = a1.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[4:8])).Mul(cl.ConcatPermuteScalars(2, 2, 3, 3, cl)))
		a2 = a2.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[8:12])).Mul(ch.ConcatPermuteScalars(0, 0, 1, 1, ch)))
		a3 = a3.Add(archsimd.LoadFloat32x4Array((*[4]float32)(in[12:16])).Mul(ch.ConcatPermuteScalars(2, 2, 3, 3, ch)))
		x, c = x[16:], c[8:]
	}
	// combine8 on both parts at once: a0+a2 is (t0, t1) and a1+a3 is (t2, t3)
	// as (re, im) pairs, their sum (t0+t2, t1+t3), and the last fold leaves
	// (re, im) in the low lanes.
	t := a0.Add(a2).Add(a1.Add(a3))
	t = t.Add(t.ConcatPermuteScalars(2, 3, 2, 3, t))
	re, im := t.GetElem(0), t.GetElem(1)
	for i, v := range c {
		re += float32(x[2*i] * v)
		im += float32(x[2*i+1] * v)
	}
	return complex(re, im)
}
