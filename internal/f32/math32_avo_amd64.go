//go:build ignore

// Generator for the assembly fallbacks used when the archsimd implementations
// in math32_simd_amd64.go are unavailable: builds without GOEXPERIMENT=simd,
// and CPUs without AVX (archsimd needs AVX even for its 128-bit types).
//
// Each vector width is a separate leaf function; the CPU dispatch happens in Go
// (math32_asm_amd64.go) so that tests can drive each path directly.
package main

import (
	"fmt"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	. "github.com/mmcloughlin/avo/reg"
)

func main() {
	// No Package() call: the signatures use only builtin types, and loading the
	// package would make the generator depend on the build constraints of the
	// files it generates for.
	ConstraintExpr("amd64")

	scaleAVX()
	scaleSSE2()
	// ConstData emits the symbol at each call site, so define each identity
	// value once and share it between the variants that use it.
	for _, op := range []reduceOp{
		{"max", "maximum", ConstData("negInfF32", U32(0xff800000)), MAXPS, VMAXPS, MAXSS},
		{"min", "minimum", ConstData("posInfF32", U32(0x7f800000)), MINPS, VMINPS, MINSS},
	} {
		reduceAVX(op)
		reduceSSE2(op)
	}
	absAVX()
	absSSE2()

	signMask := evenSignMask()
	for _, k := range []firKind{
		{"firF32", "func(dst, work, taps []float32)", "FIR", 1, false},
		{"cfirReal", "func(dst, work []complex64, taps []float32)", "CFIRReal", 2, false},
		{"cfir", "func(dst, work, taps []complex64)", "CFIR", 2, true},
	} {
		for _, isa := range []vecISA{avx, sse2} {
			firLeaf(k, isa, signMask)
		}
	}
	for _, isa := range []vecISA{avx, sse2} {
		dotLeaf(isa)
	}
	for _, isa := range []vecISA{avx, sse2} {
		cdotRealLeaf(isa)
	}

	Generate()
}

// minLen loads the two slice lengths and leaves the smaller in the returned
// register.
//
// The conditional move has to write into n, the register that is returned.
// Writing into bLen instead leaves n holding aLen unconditionally, so the
// kernel runs for len(src) elements however short dst is -- and the register
// allocator coalesces n with aLen, so the mistake disappears into a CMOV whose
// destination simply never gets read.
func minLen(aLen, bLen Register) Register {
	n := GP64()
	MOVQ(aLen, n)
	CMPQ(bLen, n)
	cmovLess(bLen, n)
	return n
}

func cmovLess(src, dst Register) { CMOVQLT(src, dst) }

// scaleBody emits dst[i] = src[i] * scale over n elements.
//
// vecs is the number of vector registers per iteration, lanes the number of
// float32 per vector. load/mul/store are the width-specific emitters.
func scaleBody(vecs, lanes int, bcast func(src Mem, dst VecVirtual), mul func(src Mem, s, dst VecVirtual), store func(src VecVirtual, dst Mem)) {
	inPtr := Load(Param("src").Base(), GP64())
	inLen := Load(Param("src").Len(), GP64())
	outPtr := Load(Param("dst").Base(), GP64())
	outLen := Load(Param("dst").Len(), GP64())

	Comment("Process min(len(src), len(dst)) elements")
	n := minLen(inLen, outLen)

	scaleAddr := NewParamAddr("scale", 48)
	s := XMM()
	MOVSS(scaleAddr, s)
	sv := XMM()
	if lanes == 8 {
		sv = YMM()
	}
	bcast(scaleAddr, sv)

	block := vecs * lanes
	i := GP64()
	XORQ(i, i)

	blockEnd := GP64()
	MOVQ(n, blockEnd)
	ANDQ(I32(^(block - 1)), blockEnd)

	Label("vector")
	CMPQ(i, blockEnd)
	JGE(LabelRef("vector1"))
	for v := range vecs {
		acc := XMM()
		if lanes == 8 {
			acc = YMM()
		}
		mul(Mem{Base: inPtr, Index: i, Scale: 4, Disp: v * lanes * 4}, sv, acc)
		store(acc, Mem{Base: outPtr, Index: i, Scale: 4, Disp: v * lanes * 4})
	}
	ADDQ(U32(block), i)
	JMP(LabelRef("vector"))

	Comment("One vector at a time, so the scalar tail is at most lanes-1 wide")
	Label("vector1")
	vecEnd := GP64()
	MOVQ(n, vecEnd)
	ANDQ(I32(^(lanes - 1)), vecEnd)
	Label("vector1loop")
	CMPQ(i, vecEnd)
	JGE(LabelRef("tail"))
	one := XMM()
	if lanes == 8 {
		one = YMM()
	}
	mul(Mem{Base: inPtr, Index: i, Scale: 4}, sv, one)
	store(one, Mem{Base: outPtr, Index: i, Scale: 4})
	ADDQ(U32(lanes), i)
	JMP(LabelRef("vector1loop"))

	Label("tail")
	CMPQ(i, n)
	JGE(LabelRef("done"))
	t := XMM()
	MOVSS(Mem{Base: inPtr, Index: i, Scale: 4}, t)
	MULSS(s, t)
	MOVSS(t, Mem{Base: outPtr, Index: i, Scale: 4})
	INCQ(i)
	JMP(LabelRef("tail"))

	Label("done")
	if lanes == 8 {
		VZEROUPPER()
	}
	RET()
}

func scaleAVX() {
	TEXT("vscaleF32AVX", NOSPLIT, "func(dst, src []float32, scale float32)")
	Pragma("noescape")
	Doc("vscaleF32AVX calculates dst[i] = src[i] * scale using AVX.")
	scaleBody(4, 8,
		avxBroadcast,
		func(src Mem, s, dst VecVirtual) { VMULPS(src, s, dst) },
		func(src VecVirtual, dst Mem) { VMOVUPS(src, dst) })
}

func scaleSSE2() {
	TEXT("vscaleF32SSE2", NOSPLIT, "func(dst, src []float32, scale float32)")
	Pragma("noescape")
	Doc("vscaleF32SSE2 calculates dst[i] = src[i] * scale using SSE2.")
	scaleBody(4, 4,
		sseBroadcast,
		func(src Mem, s, dst VecVirtual) { MOVUPS(src, dst); MULPS(s, dst) },
		func(src VecVirtual, dst Mem) { MOVUPS(src, dst) })
}

// reduceOp describes one of the two reductions. They differ only in the
// identity value seeded into the accumulators and in the instruction used, so
// the loop, the final reduction and the scalar tail below are shared.
type reduceOp struct {
	name  string          // "max" or "min"; also builds the function name
	verb  string          // "maximum" or "minimum", for the doc comment
	ident Mem             // identity: -Inf for max, +Inf for min
	ps    func(mx, x Op)  // MAXPS/MINPS
	vps   func(ops ...Op) // VMAXPS/VMINPS
	ss    func(mx, x Op)  // MAXSS/MINSS
}

// reduceBody emits the maximum or minimum of n elements, with op's identity
// value for an empty slice.
func reduceBody(op reduceOp, accs, lanes int, bcast func(src Mem, dst VecVirtual), opMem func(src Mem, dst VecVirtual), opReg func(src, dst VecVirtual), reduce func(v VecVirtual) VecVirtual) {
	inPtr := Load(Param("src").Base(), GP64())
	n := Load(Param("src").Len(), GP64())

	regs := make([]VecVirtual, accs)
	for k := range regs {
		if lanes == 8 {
			regs[k] = YMM()
		} else {
			regs[k] = XMM()
		}
		// Seed with the identity for the operation: -Inf for max, +Inf for min.
		bcast(op.ident, regs[k])
	}

	block := accs * lanes
	i := GP64()
	XORQ(i, i)
	blockEnd := GP64()
	MOVQ(n, blockEnd)
	ANDQ(I32(^(block - 1)), blockEnd)

	Label("vector")
	CMPQ(i, blockEnd)
	JGE(LabelRef("vector1"))
	for k := range accs {
		opMem(Mem{Base: inPtr, Index: i, Scale: 4, Disp: k * lanes * 4}, regs[k])
	}
	ADDQ(U32(block), i)
	JMP(LabelRef("vector"))

	Comment("One vector at a time, so the scalar tail is at most lanes-1 wide")
	Label("vector1")
	vecEnd := GP64()
	MOVQ(n, vecEnd)
	ANDQ(I32(^(lanes - 1)), vecEnd)
	Label("vector1loop")
	CMPQ(i, vecEnd)
	JGE(LabelRef("reduce"))
	opMem(Mem{Base: inPtr, Index: i, Scale: 4}, regs[0])
	ADDQ(U32(lanes), i)
	JMP(LabelRef("vector1loop"))

	Label("reduce")
	for k := 1; k < accs; k++ {
		opReg(regs[k], regs[0])
	}
	acc := reduce(regs[0])

	Label("tail")
	CMPQ(i, n)
	JGE(LabelRef("done"))
	t := XMM()
	MOVSS(Mem{Base: inPtr, Index: i, Scale: 4}, t)
	op.ss(acc, t)
	MOVAPS(t, acc)
	INCQ(i)
	JMP(LabelRef("tail"))

	Label("done")
	if lanes == 8 {
		VZEROUPPER()
	}
	Store(acc, ReturnIndex(0))
	RET()
}

func reduceAVX(op reduceOp) {
	TEXT("v"+op.name+"F32AVX", NOSPLIT, "func(src []float32) float32")
	Pragma("noescape")
	Doc(fmt.Sprintf("v%sF32AVX returns the %s value of src using AVX.", op.name, op.verb))
	reduceBody(op, 4, 8,
		avxBroadcast,
		func(src Mem, dst VecVirtual) { op.vps(src, dst, dst) },
		func(src, dst VecVirtual) { op.vps(src, dst, dst) },
		func(v VecVirtual) VecVirtual {
			lo := XMM()
			VEXTRACTF128(U8(1), v, lo)
			half := XMM()
			op.vps(lo, v.AsX(), half)
			return horizontal4(op, half)
		})
}

func reduceSSE2(op reduceOp) {
	TEXT("v"+op.name+"F32SSE2", NOSPLIT, "func(src []float32) float32")
	Pragma("noescape")
	Doc(fmt.Sprintf("v%sF32SSE2 returns the %s value of src using SSE2.", op.name, op.verb))
	reduceBody(op, 4, 4,
		sseBroadcast,
		func(src Mem, dst VecVirtual) { m := XMM(); MOVUPS(src, m); op.ps(m, dst) },
		func(src, dst VecVirtual) { op.ps(src, dst) },
		func(v VecVirtual) VecVirtual { return horizontal4(op, v) })
}

// avxBroadcast splats the float32 at src across dst.
//
// The source has to be a memory operand: VBROADCASTSS ymm, m32 is AVX, but the
// register-source form VBROADCASTSS ymm, xmm is AVX2, and these leaves are
// reached on any CPU with AVX at all (see useAVX). Loading the scalar into an
// XMM first and broadcasting from there would put an AVX2 instruction on an
// AVX1 path, where it faults with SIGILL.
func avxBroadcast(src Mem, dst VecVirtual) {
	VBROADCASTSS(src, dst)
}

// sseBroadcast splats the float32 at src across dst. MOVSS loads it into the
// low lane and zeroes the rest, then SHUFPS with a selector of zero copies that
// lane over the other three.
func sseBroadcast(src Mem, dst VecVirtual) {
	MOVSS(src, dst)
	SHUFPS(U8(0), dst, dst)
}

// horizontal4 reduces the four lanes of v to its low lane, in registers.
func horizontal4(op reduceOp, v VecVirtual) VecVirtual {
	hi := XMM()
	MOVHLPS(v, hi)
	op.ps(hi, v)
	sh := XMM()
	MOVAPS(v, sh)
	SHUFPS(U8(0x55), sh, sh)
	op.ps(sh, v)
	return v
}

// absBody emits dst[i] = |src[i]| over min(len(src), len(dst))
// complex values.
//
// vecs is the number of output vectors per iteration and lanes the number of
// float32 in one; each output vector consumes two input vectors, since a
// complex64 is two float32. magnitude takes the two input operands and returns
// the finished vector of magnitudes, which is where the two widths differ: the
// real and imaginary parts have to be separated before they can be squared, and
// the shuffle that does it is width-specific.
func absBody(vecs, lanes int, magnitude func(a, b Mem) VecVirtual, store func(src VecVirtual, dst Mem)) {
	inPtr := Load(Param("src").Base(), GP64())
	inLen := Load(Param("src").Len(), GP64())
	outPtr := Load(Param("dst").Base(), GP64())
	outLen := Load(Param("dst").Len(), GP64())

	Comment("Process min(len(src), len(dst)) complex values")
	n := minLen(inLen, outLen)

	// i counts complex values, which is also the count of output floats. The
	// input is indexed with Scale 8 and the output with Scale 4.
	block := vecs * lanes
	i := GP64()
	XORQ(i, i)

	blockEnd := GP64()
	MOVQ(n, blockEnd)
	ANDQ(I32(^(block - 1)), blockEnd)

	Label("vector")
	CMPQ(i, blockEnd)
	JGE(LabelRef("vector1"))
	for v := range vecs {
		a := Mem{Base: inPtr, Index: i, Scale: 8, Disp: v * lanes * 8}
		b := Mem{Base: inPtr, Index: i, Scale: 8, Disp: v*lanes*8 + lanes*4}
		store(magnitude(a, b), Mem{Base: outPtr, Index: i, Scale: 4, Disp: v * lanes * 4})
	}
	ADDQ(U32(block), i)
	JMP(LabelRef("vector"))

	Comment("One vector at a time, so the scalar tail is at most lanes-1 wide")
	Label("vector1")
	vecEnd := GP64()
	MOVQ(n, vecEnd)
	ANDQ(I32(^(lanes - 1)), vecEnd)
	Label("vector1loop")
	CMPQ(i, vecEnd)
	JGE(LabelRef("tail"))
	a := Mem{Base: inPtr, Index: i, Scale: 8}
	b := Mem{Base: inPtr, Index: i, Scale: 8, Disp: lanes * 4}
	store(magnitude(a, b), Mem{Base: outPtr, Index: i, Scale: 4})
	ADDQ(U32(lanes), i)
	JMP(LabelRef("vector1loop"))

	Comment("Recompute the last full vector rather than looping over the remainder")
	Label("tail")
	CMPQ(i, n)
	JGE(LabelRef("done"))
	CMPQ(n, U32(lanes))
	JL(LabelRef("scalar"))
	last := GP64()
	MOVQ(n, last)
	SUBQ(U32(lanes), last)
	av := Mem{Base: inPtr, Index: last, Scale: 8}
	bv := Mem{Base: inPtr, Index: last, Scale: 8, Disp: lanes * 4}
	store(magnitude(av, bv), Mem{Base: outPtr, Index: last, Scale: 4})
	JMP(LabelRef("done"))

	Comment("Fewer elements than one vector: one complex value at a time")
	Label("scalar")
	CMPQ(i, n)
	JGE(LabelRef("done"))
	re := XMM()
	im := XMM()
	MOVSS(Mem{Base: inPtr, Index: i, Scale: 8}, re)
	MOVSS(Mem{Base: inPtr, Index: i, Scale: 8, Disp: 4}, im)
	MULSS(re, re)
	MULSS(im, im)
	ADDSS(im, re)
	SQRTSS(re, re)
	MOVSS(re, Mem{Base: outPtr, Index: i, Scale: 4})
	INCQ(i)
	JMP(LabelRef("scalar"))

	Label("done")
	if lanes == 8 {
		VZEROUPPER()
	}
	RET()
}

func absAVX() {
	TEXT("vabsC64AVX", NOSPLIT, "func(dst []float32, src []complex64)")
	Pragma("noescape")
	Doc("vabsC64AVX calculates dst[i] = |src[i]| using AVX.")
	absBody(4, 8,
		func(a, b Mem) VecVirtual {
			// VSHUFPS selects within each 128-bit half, so the two loads are
			// first restacked by half: lo holds the four complex values that
			// become the low output lane and hi the four that become the high
			// one. Without that the deinterleave would come out permuted across
			// halves and need a lane-crossing fixup, which on AVX1 — which is
			// all useAVX promises — would cost another VPERM2F128 anyway.
			x := YMM()
			VMOVUPS(a, x)
			y := YMM()
			VMOVUPS(b, y)
			lo := YMM()
			VPERM2F128(U8(0x20), y, x, lo)
			hi := YMM()
			VPERM2F128(U8(0x31), y, x, hi)
			re := YMM()
			VSHUFPS(U8(0x88), hi, lo, re)
			im := YMM()
			VSHUFPS(U8(0xdd), hi, lo, im)
			VMULPS(re, re, re)
			VMULPS(im, im, im)
			VADDPS(im, re, re)
			VSQRTPS(re, re)
			return re
		},
		func(src VecVirtual, dst Mem) { VMOVUPS(src, dst) })
}

func absSSE2() {
	TEXT("vabsC64SSE2", NOSPLIT, "func(dst []float32, src []complex64)")
	Pragma("noescape")
	Doc("vabsC64SSE2 calculates dst[i] = |src[i]| using SSE2.")
	absBody(4, 4,
		func(a, b Mem) VecVirtual {
			// SHUFPS is SSE1 and takes its low two selectors from the
			// destination, so each half of the deinterleave needs its own copy
			// of the first load. HADDPS would do this in one instruction but is
			// SSE3, which is not part of the amd64 baseline this leaf covers.
			x := XMM()
			MOVUPS(a, x)
			y := XMM()
			MOVUPS(b, y)
			re := XMM()
			MOVAPS(x, re)
			SHUFPS(U8(0x88), y, re)
			im := XMM()
			MOVAPS(x, im)
			SHUFPS(U8(0xdd), y, im)
			MULPS(re, re)
			MULPS(im, im)
			ADDPS(im, re)
			SQRTPS(re, re)
			return re
		},
		func(src VecVirtual, dst Mem) { MOVUPS(src, dst) })
}

// vecISA is one of the two vector widths the FIR and dot-product leaves are
// generated for.
type vecISA struct {
	name  string // function name suffix
	lanes int    // float32 per vector
}

var (
	avx  = vecISA{"AVX", 8}
	sse2 = vecISA{"SSE2", 4}
)

func (v vecISA) isAVX() bool { return v.lanes == 8 }

func (v vecISA) vec() VecVirtual {
	if v.isAVX() {
		return YMM()
	}
	return XMM()
}

func (v vecISA) zero(dst VecVirtual) {
	if v.isAVX() {
		VXORPS(dst, dst, dst)
		return
	}
	XORPS(dst, dst)
}

func (v vecISA) bcast(src Mem, dst VecVirtual) {
	if v.isAVX() {
		avxBroadcast(src, dst)
		return
	}
	sseBroadcast(src, dst)
}

// load is an unaligned load.
func (v vecISA) load(src Mem, dst VecVirtual) {
	if v.isAVX() {
		VMOVUPS(src, dst)
		return
	}
	MOVUPS(src, dst)
}

func (v vecISA) store(src VecVirtual, dst Mem) {
	if v.isAVX() {
		VMOVUPS(src, dst)
		return
	}
	MOVUPS(src, dst)
}

// mulMem sets dst = s * m. SSE's MULPS faults on an unaligned memory operand,
// so that form loads first.
func (v vecISA) mulMem(m Mem, s, dst VecVirtual) {
	if v.isAVX() {
		VMULPS(m, s, dst)
		return
	}
	MOVUPS(m, dst)
	MULPS(s, dst)
}

func (v vecISA) add(src, dst VecVirtual) {
	if v.isAVX() {
		VADDPS(src, dst, dst)
		return
	}
	ADDPS(src, dst)
}

// evenSignMask defines 32 bytes with the sign bit set in every even float32
// lane: the real lanes of interleaved complex data.
func evenSignMask() Mem {
	m := GLOBL("evenSignF32", RODATA|NOPTR)
	for i := range 8 {
		var v uint32
		if i%2 == 0 {
			v = 0x80000000
		}
		DATA(4*i, U32(v))
	}
	return m
}

// firKind describes one of the three FIR leaves.
type firKind struct {
	name string // function name prefix
	sig  string
	doc  string // the wrapper in fir.go it implements
	unit int    // float32 per sample and per output
	cplx bool   // complex taps
}

// firVecs is the number of output vectors per pass over the taps. Four
// independent accumulator chains cover the latency of the adds.
const firVecs = 4

// firLeaf emits a stride-1 FIR over the float32 view of dst and work. Each
// vector lane is one output float, accumulating over the taps in order, so the
// result is the reference's whatever the width. A complex sample is two
// adjacent lanes, so with real taps the complex filter is the real one with
// samples twice as far apart.
func firLeaf(k firKind, isa vecISA, signMask Mem) {
	name := k.name + isa.name
	TEXT(name, NOSPLIT, k.sig)
	Pragma("noescape")
	Doc(fmt.Sprintf("%s implements %s for stride 1 using %s.", name, k.doc, isa.name))

	dst := Load(Param("dst").Base(), GP64())
	nf := Load(Param("dst").Len(), GP64())
	work := Load(Param("work").Base(), GP64())
	taps := Load(Param("taps").Base(), GP64())
	ntaps := Load(Param("taps").Len(), GP64())

	f := firFrame{k: k, isa: isa, dst: dst, work: work, taps: taps, ntaps: ntaps}

	Comment("Point work at the newest sample of the first output, and count floats")
	hist := GP64()
	MOVQ(ntaps, hist)
	DECQ(hist)
	if k.unit == 2 {
		SHLQ(U8(3), hist)
		SHLQ(U8(1), nf)
	} else {
		SHLQ(U8(2), hist)
	}
	ADDQ(hist, work)

	if k.cplx {
		f.mask = isa.vec()
		isa.load(signMask, f.mask)
	}

	lanes := isa.lanes
	i := GP64()
	XORQ(i, i)
	end := GP64()
	MOVQ(nf, end)
	ANDQ(I32(^(firVecs*lanes - 1)), end)

	Label("block")
	CMPQ(i, end)
	JGE(LabelRef("single"))
	f.vectors(firVecs, i, "blocktaps")
	ADDQ(U32(firVecs*lanes), i)
	JMP(LabelRef("block"))

	Label("single")
	MOVQ(nf, end)
	ANDQ(I32(^(lanes - 1)), end)
	Label("singleloop")
	CMPQ(i, end)
	JGE(LabelRef("tail"))
	f.vectors(1, i, "singletaps")
	ADDQ(U32(lanes), i)
	JMP(LabelRef("singleloop"))

	Comment("Recompute the last full vector: every lane is an independent output")
	Label("tail")
	CMPQ(i, nf)
	JGE(LabelRef("done"))
	CMPQ(nf, U32(lanes))
	JL(LabelRef("scalar"))
	MOVQ(nf, i)
	SUBQ(U32(lanes), i)
	f.vectors(1, i, "tailtaps")
	JMP(LabelRef("done"))

	Comment("Fewer floats than one vector: one output at a time")
	Label("scalar")
	if isa.isAVX() {
		VZEROUPPER()
	}
	var maskX VecVirtual
	if k.cplx {
		maskX = XMM()
		MOVUPS(signMask, maskX)
	}
	Label("scalarloop")
	CMPQ(i, nf)
	JGE(LabelRef("scalardone"))
	f.scalar(i, maskX)
	ADDQ(U32(k.unit), i)
	JMP(LabelRef("scalarloop"))
	Label("scalardone")
	RET()

	Label("done")
	if isa.isAVX() {
		VZEROUPPER()
	}
	RET()
}

// firFrame holds the registers shared by the parts of one FIR leaf.
type firFrame struct {
	k                      firKind
	isa                    vecISA
	dst, work, taps, ntaps Register
	mask                   VecVirtual // evenSignF32, for complex taps
}

// start sets up a pass over the taps for the outputs starting at float i:
// the newest sample, the first tap and the tap count.
func (f firFrame) start(i Register) (wp, tp, cnt Register) {
	wp = GP64()
	LEAQ(Mem{Base: f.work, Index: i, Scale: 4}, wp)
	tp = GP64()
	MOVQ(f.taps, tp)
	cnt = GP64()
	MOVQ(f.ntaps, cnt)
	return wp, tp, cnt
}

// next steps to the next tap and the next older sample.
func (f firFrame) next(wp, tp, cnt Register, loop string) {
	tapSize := 4
	if f.k.cplx {
		tapSize = 8
	}
	ADDQ(U32(tapSize), tp)
	SUBQ(U32(4*f.k.unit), wp)
	DECQ(cnt)
	JNZ(LabelRef(loop))
}

// vectors computes vecs vectors of output floats starting at float i.
func (f firFrame) vectors(vecs int, i Register, loop string) {
	isa := f.isa
	wp, tp, cnt := f.start(i)
	accs := make([]VecVirtual, vecs)
	for v := range accs {
		accs[v] = isa.vec()
		isa.zero(accs[v])
	}
	Label(loop)
	if !f.k.cplx {
		c := isa.vec()
		isa.bcast(Mem{Base: tp}, c)
		for v, acc := range accs {
			p := isa.vec()
			isa.mulMem(Mem{Base: wp, Disp: v * isa.lanes * 4}, c, p)
			isa.add(p, acc)
		}
	} else {
		// p1 = v*cr and p2 = swap(v)*ci with the real lanes negated, so p1+p2
		// is (cr*vr - ci*vi, cr*vi + ci*vr) with each product rounded. The
		// sign goes on ci once per tap: (-ci)*vi rounds to exactly -(ci*vi).
		cr := isa.vec()
		isa.bcast(Mem{Base: tp}, cr)
		ci := isa.vec()
		isa.bcast(Mem{Base: tp, Disp: 4}, ci)
		if isa.isAVX() {
			VXORPS(f.mask, ci, ci)
		} else {
			XORPS(f.mask, ci)
		}
		for v, acc := range accs {
			m := Mem{Base: wp, Disp: v * isa.lanes * 4}
			p := isa.vec()
			s := isa.vec()
			if isa.isAVX() {
				VMULPS(m, cr, p)
				VPERMILPS(U8(0xb1), m, s)
				VMULPS(s, ci, s)
			} else {
				MOVUPS(m, p)
				MOVAPS(p, s)
				SHUFPS(U8(0xb1), s, s)
				MULPS(cr, p)
				MULPS(ci, s)
			}
			isa.add(s, p)
			isa.add(p, acc)
		}
	}
	f.next(wp, tp, cnt, loop)
	for v, acc := range accs {
		isa.store(acc, Mem{Base: f.dst, Index: i, Scale: 4, Disp: v * isa.lanes * 4})
	}
}

// scalar computes the one output starting at float i in the low one or two
// lanes of an xmm, with SSE: an AVX leaf has executed VZEROUPPER first.
func (f firFrame) scalar(i Register, mask VecVirtual) {
	wp, tp, cnt := f.start(i)
	acc := XMM()
	XORPS(acc, acc)
	Label("scalartaps")
	switch {
	case f.k.unit == 1:
		p := XMM()
		MOVSS(Mem{Base: wp}, p)
		MULSS(Mem{Base: tp}, p)
		ADDSS(p, acc)
	case !f.k.cplx:
		p := XMM()
		MOVSD(Mem{Base: wp}, p)
		c := XMM()
		MOVSS(Mem{Base: tp}, c)
		UNPCKLPS(c, c)
		MULPS(c, p)
		ADDPS(p, acc)
	default:
		p := XMM()
		MOVSD(Mem{Base: wp}, p)
		s := XMM()
		MOVAPS(p, s)
		SHUFPS(U8(0xb1), s, s)
		cr := XMM()
		sseBroadcast(Mem{Base: tp}, cr)
		ci := XMM()
		sseBroadcast(Mem{Base: tp, Disp: 4}, ci)
		XORPS(mask, ci)
		MULPS(cr, p)
		MULPS(ci, s)
		ADDPS(s, p)
		ADDPS(p, acc)
	}
	f.next(wp, tp, cnt, "scalartaps")
	out := Mem{Base: f.dst, Index: i, Scale: 4}
	if f.k.unit == 1 {
		MOVSS(acc, out)
	} else {
		MOVSD(acc, out)
	}
}

// reduce8 adds accumulators 4-7 to 0-3, leaving the four sums in an xmm. The
// eight are the two halves of the ymm t, or the xmm t (0-3) and u (4-7). An
// AVX leaf executes VZEROUPPER here: everything after it is SSE.
func reduce8(isa vecISA, t, u VecVirtual) VecVirtual {
	if isa.isAVX() {
		hi := XMM()
		VEXTRACTF128(U8(1), t, hi)
		s := XMM()
		VADDPS(hi, t.AsX(), s)
		VZEROUPPER()
		return s
	}
	ADDPS(u, t)
	return t
}

// dotLeaf emits Dot: eight accumulators, one ymm or two xmm, combined as
// combine8 does, then the remainder added in order.
func dotLeaf(isa vecISA) {
	name := "dotF32" + isa.name
	TEXT(name, NOSPLIT, "func(x, c []float32) float32")
	Pragma("noescape")
	Doc(fmt.Sprintf("%s implements Dot using %s. x must be at least as long as c.", name, isa.name))
	x := Load(Param("x").Base(), GP64())
	c := Load(Param("c").Base(), GP64())
	n := Load(Param("c").Len(), GP64())

	accs := make([]VecVirtual, 8/isa.lanes)
	for k := range accs {
		accs[k] = isa.vec()
		isa.zero(accs[k])
	}
	i := GP64()
	XORQ(i, i)
	end := GP64()
	MOVQ(n, end)
	ANDQ(I32(-8), end)

	Label("block")
	CMPQ(i, end)
	JGE(LabelRef("reduce"))
	for k, acc := range accs {
		cv := isa.vec()
		isa.load(Mem{Base: c, Index: i, Scale: 4, Disp: k * isa.lanes * 4}, cv)
		p := isa.vec()
		isa.mulMem(Mem{Base: x, Index: i, Scale: 4, Disp: k * isa.lanes * 4}, cv, p)
		isa.add(p, acc)
	}
	ADDQ(U32(8), i)
	JMP(LabelRef("block"))

	Label("reduce")
	var u VecVirtual
	if !isa.isAVX() {
		u = accs[1]
	}
	t := reduce8(isa, accs[0], u)
	hi := XMM()
	MOVHLPS(t, hi)
	ADDPS(hi, t)
	s := XMM()
	MOVAPS(t, s)
	SHUFPS(U8(0x55), s, s)
	ADDSS(s, t)

	Label("tail")
	CMPQ(i, n)
	JGE(LabelRef("done"))
	p := XMM()
	MOVSS(Mem{Base: x, Index: i, Scale: 4}, p)
	MULSS(Mem{Base: c, Index: i, Scale: 4}, p)
	ADDSS(p, t)
	INCQ(i)
	JMP(LabelRef("tail"))

	Label("done")
	Store(t, ReturnIndex(0))
	RET()
}

// cdotRealLeaf emits CDotReal: Dot over each part of interleaved complex
// samples, eight samples per block. The coefficients are duplicated into
// adjacent lanes to line up with the samples, so lane pair l of the
// accumulators holds accumulator l of both parts, and the combine is Dot's
// tree two lanes at a time.
func cdotRealLeaf(isa vecISA) {
	name := "cdotReal" + isa.name
	TEXT(name, NOSPLIT, "func(x []complex64, c []float32) complex64")
	Pragma("noescape")
	Doc(fmt.Sprintf("%s implements CDotReal using %s. x must be at least as long as c.", name, isa.name))
	x := Load(Param("x").Base(), GP64())
	c := Load(Param("c").Base(), GP64())
	n := Load(Param("c").Len(), GP64())

	// Each accumulator vector holds lanes/2 samples' worth.
	accs := make([]VecVirtual, 16/isa.lanes)
	for k := range accs {
		accs[k] = isa.vec()
		isa.zero(accs[k])
	}
	i := GP64()
	XORQ(i, i)
	end := GP64()
	MOVQ(n, end)
	ANDQ(I32(-8), end)

	Label("block")
	CMPQ(i, end)
	JGE(LabelRef("reduce"))
	if isa.isAVX() {
		// The unpacks work within each 128-bit half, giving
		// [c0 c0 c1 c1 | c4 c4 c5 c5] and [c2 c2 c3 c3 | c6 c6 c7 c7];
		// VPERM2F128 regroups the halves in sample order.
		cv := YMM()
		VMOVUPS(Mem{Base: c, Index: i, Scale: 4}, cv)
		lo := YMM()
		VUNPCKLPS(cv, cv, lo)
		hi := YMM()
		VUNPCKHPS(cv, cv, hi)
		c0 := YMM()
		VPERM2F128(U8(0x20), hi, lo, c0)
		c1 := YMM()
		VPERM2F128(U8(0x31), hi, lo, c1)
		for k, cd := range []VecVirtual{c0, c1} {
			p := YMM()
			VMULPS(Mem{Base: x, Index: i, Scale: 8, Disp: 32 * k}, cd, p)
			VADDPS(p, accs[k], accs[k])
		}
	} else {
		for h := range 2 {
			cv := XMM()
			MOVUPS(Mem{Base: c, Index: i, Scale: 4, Disp: 16 * h}, cv)
			lo := XMM()
			MOVAPS(cv, lo)
			UNPCKLPS(lo, lo)
			UNPCKHPS(cv, cv)
			for k, cd := range []VecVirtual{lo, cv} {
				p := XMM()
				isa.mulMem(Mem{Base: x, Index: i, Scale: 8, Disp: 32*h + 16*k}, cd, p)
				ADDPS(p, accs[2*h+k])
			}
		}
	}
	ADDQ(U32(8), i)
	JMP(LabelRef("block"))

	// The first half of accs holds accumulators 0-3 of both parts and the
	// second half 4-7.
	Label("reduce")
	var t VecVirtual
	if isa.isAVX() {
		VADDPS(accs[1], accs[0], accs[0])
		t = reduce8(isa, accs[0], nil)
	} else {
		ADDPS(accs[2], accs[0])
		ADDPS(accs[3], accs[1])
		t = reduce8(isa, accs[0], accs[1])
	}
	hi := XMM()
	MOVHLPS(t, hi)
	ADDPS(hi, t)

	Comment("The real part is in lane 0 and the imaginary part in lane 1")
	Label("tail")
	CMPQ(i, n)
	JGE(LabelRef("done"))
	p := XMM()
	MOVSD(Mem{Base: x, Index: i, Scale: 8}, p)
	cs := XMM()
	MOVSS(Mem{Base: c, Index: i, Scale: 4}, cs)
	UNPCKLPS(cs, cs)
	MULPS(cs, p)
	ADDPS(p, t)
	INCQ(i)
	JMP(LabelRef("tail"))

	Label("done")
	Store(t, ReturnIndex(0).Real())
	im := XMM()
	MOVAPS(t, im)
	SHUFPS(U8(0x55), im, im)
	Store(im, ReturnIndex(0).Imag())
	RET()
}
