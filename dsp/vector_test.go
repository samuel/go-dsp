package dsp

import (
	"fmt"
	"math"
	"math/cmplx"
	"testing"

	"github.com/samuel/go-dsp/dsp/f32"
)

// TestVectorCoversEveryWidth instantiates every member of Float and Complex
// against every generic entry point and checks a known result. The dispatch
// switches have no default arm, so a constraint member added without a matching
// arm compiles and writes nothing; checking a real answer is what catches it.
func TestVectorCoversEveryWidth(t *testing.T) {
	t.Run("float32", func(t *testing.T) { checkRealFamily[float32](t) })
	t.Run("float64", func(t *testing.T) { checkRealFamily[float64](t) })
	t.Run("complex64", func(t *testing.T) { checkComplexFamily[complex64](t) })
	t.Run("complex128", func(t *testing.T) { checkComplexFamily[complex128](t) })
}

func checkRealFamily[F Float](t *testing.T) {
	a := []F{1, 2, 3, 4, 5, 6, 7, 8, 9}
	b := []F{2, 2, 2, 2, 2, 2, 2, 2, 2}
	dst := make([]F, len(a))

	VScale(dst, a, 3)
	wantSlice(t, "VScale", dst, []F{3, 6, 9, 12, 15, 18, 21, 24, 27})

	VAdd(dst, a, b)
	wantSlice(t, "VAdd", dst, []F{3, 4, 5, 6, 7, 8, 9, 10, 11})

	VSub(dst, a, b)
	wantSlice(t, "VSub", dst, []F{-1, 0, 1, 2, 3, 4, 5, 6, 7})

	VMul(dst, a, b)
	wantSlice(t, "VMul", dst, []F{2, 4, 6, 8, 10, 12, 14, 16, 18})

	if got := VMax(a); got != 9 {
		t.Errorf("VMax = %v, want 9", got)
	}
	if got := VMin(a); got != 1 {
		t.Errorf("VMin = %v, want 1", got)
	}
	if got := VMax([]F{}); !math.IsInf(float64(got), -1) {
		t.Errorf("VMax(empty) = %v, want -Inf", got)
	}
	if got := VMin([]F{}); !math.IsInf(float64(got), 1) {
		t.Errorf("VMin(empty) = %v, want +Inf", got)
	}

	// Sum is exact here because every partial sum is: the accumulator order
	// changes which additions happen, not the result, when nothing rounds.
	if got := VSum(a); got != 45 {
		t.Errorf("VSum = %v, want 45", got)
	}
	if got := VSum([]F{}); got != 0 {
		t.Errorf("VSum(empty) = %v, want 0", got)
	}
	if v, i := VMaxIdx(a); v != 9 || i != 8 {
		t.Errorf("VMaxIdx = (%v, %d), want (9, 8)", v, i)
	}
	if v, i := VMaxIdx([]F{3, 7, 7, 1}); v != 7 || i != 1 {
		t.Errorf("VMaxIdx tie = (%v, %d), want (7, 1)", v, i)
	}
	if v, i := VMaxIdx([]F{}); !math.IsInf(float64(v), -1) || i != -1 {
		t.Errorf("VMaxIdx(empty) = (%v, %d), want (-Inf, -1)", v, i)
	}

	// In place, which is the three-operand form with dst aliasing an input.
	acc := []F{1, 2, 3}
	VAdd(acc, acc, []F{10, 20, 30})
	wantSlice(t, "VAdd in place", acc, []F{11, 22, 33})
}

func checkComplexFamily[C Complex](t *testing.T) {
	a := []C{complex(1, 2), complex(-3, 4), complex(5, -6)}
	b := []C{complex(0, 1), complex(2, 0), complex(1, 1)}
	dst := make([]C, len(a))

	VCAdd(dst, a, b)
	wantSlice(t, "VCAdd", dst, []C{complex(1, 3), complex(-1, 4), complex(6, -5)})

	VCSub(dst, a, b)
	wantSlice(t, "VCSub", dst, []C{complex(1, 1), complex(-5, 4), complex(4, -7)})

	VCMul(dst, a, b)
	wantSlice(t, "VCMul", dst, []C{complex(-2, 1), complex(-6, 8), complex(11, -1)})

	VCScale(dst, a, complex(0, 1))
	wantSlice(t, "VCScale", dst, []C{complex(-2, 1), complex(-4, -3), complex(6, 5)})

	VCScaleReal(dst, a, 2.0)
	wantSlice(t, "VCScaleReal", dst, []C{complex(2, 4), complex(-6, 8), complex(10, -12)})

	VCConj(dst, a)
	wantSlice(t, "VCConj", dst, []C{complex(1, -2), complex(-3, -4), complex(5, 6)})

	if got, want := Conj(a[0]), C(complex(1, -2)); got != want {
		t.Errorf("Conj = %v, want %v", got, want)
	}
	if got, want := Phase(C(complex(0, 1))), math.Pi/2; math.Abs(got-want) > 1e-12 {
		t.Errorf("Phase = %v, want %v", got, want)
	}
	if got, want := PolarDiscriminator(C(complex(0, 1)), C(complex(1, 0))), math.Pi/2; math.Abs(got-want) > 1e-6 {
		t.Errorf("PolarDiscriminator = %v, want %v", got, want)
	}

	// The cross-type entry points, at both destination widths. The mismatched
	// pair must equal the matched one widened, not a more accurate answer.
	mag32 := make([]float32, len(a))
	mag64 := make([]float64, len(a))
	VCAbs(mag32, a)
	VCAbs(mag64, a)
	for i, v := range a {
		want := cmplx.Abs(complex128(v))
		if math.Abs(float64(mag32[i])-want) > 1e-6 {
			t.Errorf("VCAbs[float32][%d] = %v, want %v", i, mag32[i], want)
		}
		if math.Abs(mag64[i]-want) > 1e-6 {
			t.Errorf("VCAbs[float64][%d] = %v, want %v", i, mag64[i], want)
		}
		if float64(mag32[i]) != mag64[i] && isC64[C]() {
			t.Errorf("VCAbs widths disagree at %d: %v vs %v", i, mag32[i], mag64[i])
		}
	}

	// VCMagSq is VCAbs before the square root, at both destination widths.
	sq32 := make([]float32, len(a))
	sq64 := make([]float64, len(a))
	VCMagSq(sq32, a)
	VCMagSq(sq64, a)
	for i, v := range a {
		z := complex128(v)
		want := real(z)*real(z) + imag(z)*imag(z)
		if math.Abs(float64(sq32[i])-want) > 1e-4 {
			t.Errorf("VCMagSq[float32][%d] = %v, want %v", i, sq32[i], want)
		}
		if math.Abs(sq64[i]-want) > 1e-9 {
			t.Errorf("VCMagSq[float64][%d] = %v, want %v", i, sq64[i], want)
		}
		// VCAbs and VCMagSq describe the same quantity, so squaring one gives
		// the other -- to the precision of the width the arithmetic ran at,
		// which for complex64 is float32 on both sides of the square root.
		tol := 1e-12
		if isC64[C]() {
			tol = 1e-5
		}
		if got := float64(mag64[i] * mag64[i]); math.Abs(got-sq64[i]) > tol*math.Abs(sq64[i]) {
			t.Errorf("VCAbs^2 [%d] = %v, VCMagSq = %v", i, got, sq64[i])
		}
	}

	w32 := []float32{2, 3, 4}
	w64 := []float64{2, 3, 4}
	want := []C{complex(2, 4), complex(-9, 12), complex(20, -24)}
	VCMulReal(dst, a, w32)
	wantSlice(t, "VCMulReal float32 weights", dst, want)
	VCMulReal(dst, a, w64)
	wantSlice(t, "VCMulReal float64 weights", dst, want)
}

// isC64 reports whether C is complex64, which decides both the tolerance above
// and whether the two VCAbs destination widths are defined to agree bit for bit.
func isC64[C Complex]() bool {
	var z C
	_, ok := any(z).(complex64)
	return ok
}

func wantSlice[T comparable](t *testing.T, name string, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %d elements, want %d (%v)", name, len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %v, want %v", name, i, got[i], want[i])
		}
	}
}

// TestVectorLengthAndAliasing pins the two rules every entry point shares: the
// shortest slice bounds the work, and nothing is written past it.
func TestVectorLengthAndAliasing(t *testing.T) {
	dst := make([]float64, 5)
	VAdd(dst, []float64{1, 2, 3}, []float64{10, 20, 30, 40})
	wantSlice(t, "VAdd short", dst, []float64{11, 22, 33, 0, 0})

	cdst := make([]complex128, 4)
	VCAdd(cdst, []complex128{1, 2}, []complex128{10, 20, 30})
	wantSlice(t, "VCAdd short", cdst, []complex128{11, 22, 0, 0})

	// Scaling in place, dst aliasing src exactly.
	x := []float32{1, 2, 3, 4}
	VScale(x, x, 2)
	wantSlice(t, "VScale in place", x, []float32{2, 4, 6, 8})

	c := []complex64{complex(1, 1), complex(2, 2)}
	VCConj(c, c)
	wantSlice(t, "VCConj in place", c, []complex64{complex(1, -1), complex(2, -2)})
}

// TestVectorMatchesScalar checks every generic entry point against a scalar
// loop written here, over lengths that cross the kernels' unrolled blocks and
// land on awkward tails. It deliberately does not compare against the kernel
// the wrapper dispatches to: two vectorized implementations checked against
// each other agree on the tail bugs they share, which is the failure a length
// sweep exists to catch.
//
// The comparison is exact, which is what makes the data integral. Go may
// contract a product and a sum into one FMA and arm64 does, so a magnitude or a
// complex product written here need not round the way a kernel elsewhere
// rounds it. Integers this small keep every square, product and sum exact even
// in float32, which leaves only the square root -- exactly rounded by IEEE --
// so every correct implementation has to agree bit for bit. The general-data
// case below covers the operations where exactness needs no such help.
func TestVectorMatchesScalar(t *testing.T) {
	t.Run("float32", func(t *testing.T) { checkAgainstScalar[float32, complex64](t) })
	t.Run("float64", func(t *testing.T) { checkAgainstScalar[float64, complex128](t) })
}

// vectorSweepLengths crosses the widest unrolled block any kernel uses (64
// float32s) twice, and lands one either side of it.
var vectorSweepLengths = []int{0, 1, 3, 7, 8, 15, 16, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 257}

func checkAgainstScalar[F Float, C Complex](t *testing.T) {
	const k = 1.25 // dyadic, so scaling an integer stays exact

	for _, n := range vectorSweepLengths {
		a := make([]F, n)
		b := make([]F, n)
		ca := make([]C, n)
		cb := make([]C, n)
		for i := range n {
			a[i] = F(i%17 - 8)
			b[i] = F(i%7 - 3)
			ca[i] = C(complex(float64(i%13-6), float64(i%11-5)))
			cb[i] = C(complex(float64(i%5-2), float64(i%9-4)))
		}
		at := func(name string) string { return fmt.Sprintf("%s n=%d", name, n) }

		real1 := func(name string, fn func(dst, src []F), scalar func(F) F) {
			want := make([]F, n)
			for i, v := range a {
				want[i] = scalar(v)
			}
			got := make([]F, n)
			fn(got, a)
			wantSlice(t, at(name), got, want)
		}
		real2 := func(name string, fn func(dst, x, y []F), scalar func(x, y F) F) {
			want := make([]F, n)
			for i := range a {
				want[i] = scalar(a[i], b[i])
			}
			got := make([]F, n)
			fn(got, a, b)
			wantSlice(t, at(name), got, want)
		}
		cplx1 := func(name string, fn func(dst, src []C), scalar func(C) C) {
			want := make([]C, n)
			for i, v := range ca {
				want[i] = scalar(v)
			}
			got := make([]C, n)
			fn(got, ca)
			wantSlice(t, at(name), got, want)
		}
		cplx2 := func(name string, fn func(dst, x, y []C), scalar func(x, y C) C) {
			want := make([]C, n)
			for i := range ca {
				want[i] = scalar(ca[i], cb[i])
			}
			got := make([]C, n)
			fn(got, ca, cb)
			wantSlice(t, at(name), got, want)
		}

		real1("VScale", func(dst, src []F) { VScale(dst, src, k) }, func(v F) F { return v * k })
		real2("VAdd", VAdd[F], func(x, y F) F { return x + y })
		real2("VSub", VSub[F], func(x, y F) F { return x - y })
		real2("VMul", VMul[F], func(x, y F) F { return x * y })

		cplx1("VCConj", VCConj[C], func(v C) C { return C(complex(real(complex128(v)), -imag(complex128(v)))) })
		cplx2("VCAdd", VCAdd[C], func(x, y C) C { return x + y })
		cplx2("VCSub", VCSub[C], func(x, y C) C { return x - y })
		cplx2("VCMul", VCMul[C], func(x, y C) C { return x * y })
		cplx1("VCScale", func(dst, src []C) { VCScale(dst, src, C(complex(2, -3))) },
			func(v C) C { return v * C(complex(2, -3)) })
		cplx1("VCScaleReal", func(dst, src []C) { VCScaleReal(dst, src, F(k)) },
			func(v C) C {
				z := complex128(v)
				return C(complex(real(z)*k, imag(z)*k))
			})

		// The cross-type pair, at the destination width that matches the sample
		// width. That is the pairing the assembly and archsimd kernels back;
		// the mismatched pairing is a scalar loop and is covered above.
		mag := make([]F, n)
		magSq := make([]F, n)
		wantMag := make([]F, n)
		wantSq := make([]F, n)
		for i, v := range ca {
			z := complex128(v)
			sq := real(z)*real(z) + imag(z)*imag(z)
			wantSq[i] = F(sq)
			wantMag[i] = F(math.Sqrt(sq))
		}
		VCAbs(mag, ca)
		VCMagSq(magSq, ca)
		wantSlice(t, at("VCAbs"), mag, wantMag)
		wantSlice(t, at("VCMagSq"), magSq, wantSq)

		wmul := make([]C, n)
		wantMul := make([]C, n)
		for i, v := range ca {
			z := complex128(v)
			wantMul[i] = C(complex(real(z)*float64(b[i]), imag(z)*float64(b[i])))
		}
		VCMulReal(wmul, ca, b)
		wantSlice(t, at("VCMulReal"), wmul, wantMul)

		if n > 0 {
			var wantMax, wantMin, wantSum F = a[0], a[0], 0
			wantIdx := 0
			for i, v := range a {
				if v > wantMax {
					wantMax, wantIdx = v, i
				}
				wantMin = min(wantMin, v)
				wantSum += v
			}
			if got := VMax(a); got != wantMax {
				t.Errorf("VMax n=%d = %v, want %v", n, got, wantMax)
			}
			if got := VMin(a); got != wantMin {
				t.Errorf("VMin n=%d = %v, want %v", n, got, wantMin)
			}
			if got := VSum(a); got != wantSum {
				t.Errorf("VSum n=%d = %v, want %v", n, got, wantSum)
			}
			if v, i := VMaxIdx(a); v != wantMax || i != wantIdx {
				t.Errorf("VMaxIdx n=%d = (%v, %d), want (%v, %d)", n, v, i, wantMax, wantIdx)
			}
		}
	}

	// The operations that are one rounded arithmetic step per element are exact
	// against any data, so those get a sweep with nothing convenient about the
	// values. A magnitude or a complex product is not one step and is left out.
	for _, n := range vectorSweepLengths {
		a := make([]F, n)
		b := make([]F, n)
		for i := range n {
			a[i] = F(math.Sin(float64(i)*0.7) * 1e3)
			b[i] = F(math.Cos(float64(i)*1.3) * 7e-4)
		}
		wantAdd := make([]F, n)
		wantMul := make([]F, n)
		wantScale := make([]F, n)
		for i := range a {
			wantAdd[i] = a[i] + b[i]
			wantMul[i] = a[i] * b[i]
			wantScale[i] = a[i] * 1.0001
		}
		got := make([]F, n)
		VAdd(got, a, b)
		wantSlice(t, fmt.Sprintf("VAdd general n=%d", n), got, wantAdd)
		VMul(got, a, b)
		wantSlice(t, fmt.Sprintf("VMul general n=%d", n), got, wantMul)
		VScale(got, a, 1.0001)
		wantSlice(t, fmt.Sprintf("VScale general n=%d", n), got, wantScale)
	}
}

// TestDispatchDoesNotAllocate is the half of the dispatch contract that could
// regress silently. The interface value the type switch builds must not escape;
// if it ever does, every call starts allocating and only a benchmark would show
// it.
func TestDispatchDoesNotAllocate(t *testing.T) {
	f32s, f64s := make([]float32, 64), make([]float64, 64)
	c64, c128 := make([]complex64, 64), make([]complex128, 64)
	w32 := make([]float32, 64)

	cases := []struct {
		name string
		fn   func()
	}{
		{"VScale/f32", func() { VScale(f32s, f32s, 0.5) }},
		{"VScale/f64", func() { VScale(f64s, f64s, 0.5) }},
		{"VMax/f32", func() { _ = VMax(f32s) }},
		{"VMax/f64", func() { _ = VMax(f64s) }},
		{"VMin/f32", func() { _ = VMin(f32s) }},
		{"VCMul/c64", func() { VCMul(c64, c64, c64) }},
		{"VCMul/c128", func() { VCMul(c128, c128, c128) }},
		{"VCScale/c64", func() { VCScale(c64, c64, 2) }},
		{"VCScaleReal/c64", func() { VCScaleReal(c64, c64, 0.5) }},
		{"VCScaleReal/c128", func() { VCScaleReal(c128, c128, 0.5) }},
		{"VCConj/c64", func() { VCConj(c64, c64) }},
		{"VCConj/c128", func() { VCConj(c128, c128) }},
		{"VCAbs/c64->f32", func() { VCAbs(w32, c64) }},
		{"VCAbs/c128->f64", func() { VCAbs(f64s, c128) }},
		{"VCMulReal/c64", func() { VCMulReal(c64, c64, w32) }},
		{"VSum/f32", func() { _ = VSum(f32s) }},
		{"VSum/f64", func() { _ = VSum(f64s) }},
		{"VMaxIdx/f32", func() { _, _ = VMaxIdx(f32s) }},
		{"VMaxIdx/f64", func() { _, _ = VMaxIdx(f64s) }},
		{"VCMagSq/c64->f32", func() { VCMagSq(w32, c64) }},
		{"VCMagSq/c128->f64", func() { VCMagSq(f64s, c128) }},
	}
	for _, c := range cases {
		if n := testing.AllocsPerRun(50, c.fn); n != 0 {
			t.Errorf("%s: %v allocs per call, want 0", c.name, n)
		}
	}
}

// BenchmarkVScale measures the generic entry point against the concrete kernel
// it dispatches to. Both live in this one binary deliberately: comparing across
// binaries is unreliable at the few-percent level, and the whole question here
// is a few percent.
//
// The dispatch does not compile away -- Go stencils by GC shape and reads the
// concrete type from a dictionary -- so expect a fixed per-call cost rather
// than a per-element one. f32.BenchmarkScale measures the kernel on its own,
// over the same size sweep, so the two are directly comparable.
func BenchmarkVScale(b *testing.B) {
	for _, sz := range simdBenchSizes {
		src := make([]float32, sz.n)
		dst := make([]float32, sz.n)
		b.Run("generic/"+sz.name, func(b *testing.B) {
			b.SetBytes(int64(sz.n * 4))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				VScale(dst, src, 1.0001)
			}
		})
		b.Run("kernel/"+sz.name, func(b *testing.B) {
			b.SetBytes(int64(sz.n * 4))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				f32.Scale(dst, src, 1.0001)
			}
		})
	}
}
