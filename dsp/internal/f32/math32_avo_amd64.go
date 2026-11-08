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
	Doc("vscaleF32AVX calculates dst[i] = src[i] * scale using AVX.")
	scaleBody(4, 8,
		avxBroadcast,
		func(src Mem, s, dst VecVirtual) { VMULPS(src, s, dst) },
		func(src VecVirtual, dst Mem) { VMOVUPS(src, dst) })
}

func scaleSSE2() {
	TEXT("vscaleF32SSE2", NOSPLIT, "func(dst, src []float32, scale float32)")
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
