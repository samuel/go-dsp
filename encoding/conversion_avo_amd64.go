//go:build ignore

// Generator for the assembly used whenever the archsimd implementation in
// conversion_simd_amd64.go is unavailable: builds without GOEXPERIMENT=simd,
// and CPUs without AVX (archsimd needs AVX even for its 128-bit types).
//
// Each CPU feature is a separate leaf function; the dispatch happens in Go
// (conversion_asm_amd64.go) so that tests can drive each path directly.
package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	. "github.com/mmcloughlin/avo/reg"
)

func main() {
	// No Package() call: the signatures use only builtin types, and loading the
	// package would make the generator depend on the build constraints of the
	// files it generates for.
	ConstraintExpr("amd64")

	u8ToF32Leaf("SSE4", "SSE4.1", u8ToF32SSE4Block)
	u8ToF32Leaf("SSE2", "SSE2", u8ToF32SSE2Block)

	Generate()
}

// u8ToF32Leaf emits one complete conversion: shortest length, the alignment
// prologue, block is the vectorized body, then the scalar stepper for what is
// left.
func u8ToF32Leaf(suffix, feature string, block func(inputPtr, outputPtr, index, maxIndex Register)) {
	TEXT("u8ToF32"+suffix, NOSPLIT, "func(dst []float32, src []byte)")
	Doc("u8ToF32" + suffix + " converts unsigned 8-bit samples to 32-bit float using " + feature + ".")
	inputPtr := Load(Param("src").Base(), GP64())
	inputLen := Load(Param("src").Len(), GP64())
	outputPtr := Load(Param("dst").Base(), GP64())
	outputLen := Load(Param("dst").Len(), GP64())

	Comment("Pick shortest length")
	CMPQ(outputLen, inputLen)
	JGE(LabelRef("u8tof32_min_len"))
	MOVQ(outputLen, inputLen)
	Label("u8tof32_min_len")

	index := GP64()
	MOVQ(U64(0), index)

	t64 := GP64()

	Comment("If src is too short to optimize (less than 32 bytes) then single step")
	MOVQ(U64(32), t64)
	CMPQ(t64, inputLen)
	JGE(LabelRef("u8tof32_stepper"))

	Comment("Align dst to 16-byte boundary")
	MOVQ(outputPtr, t64)
	ANDQ(Imm(0xf), t64)
	SHRQ(Imm(2), t64) // divide by 4 to convert bytes to 32-bit blocks
	JZ(LabelRef("u8tof32_aligned"))

	Comment("t64 is how many 32-bit slots dst already sits past a boundary, so 4-t64 is how many to step first")
	t2 := GP64()
	MOVQ(U64(4), t2)
	SUBQ(t64, t2)
	u8ToF32Step(inputPtr, outputPtr, index, t2, "u8tof32_align")

	Label("u8tof32_aligned")

	Comment("Bound the blocks relative to the index the prologue left behind, not absolutely: an absolute len&^15 lets the last block start up to 15 elements short of the end and run 15 past it")
	n := GP64()
	MOVQ(inputLen, n)
	SUBQ(index, n)
	ANDQ(U32(^uint32(15)), n)
	ADDQ(index, n)
	CMPQ(index, n)
	JGE(LabelRef("u8tof32_stepper"))

	block(inputPtr, outputPtr, index, n)

	Comment("TODO: work increasingly smaller blocks")

	Label("u8tof32_stepper")
	CMPQ(index, inputLen)
	JGE(LabelRef("u8tof32_done"))

	u8ToF32Step(inputPtr, outputPtr, index, inputLen, "u8tof32_step")

	Label("u8tof32_done")
	RET()
}

func u8ToF32Step(inputPtr, outputPtr, index, maxIndex Register, label string) {
	Label(label)
	x0 := XMM()
	t64 := GP64()
	MOVBQZX(Mem{Base: inputPtr}, t64)
	INCQ(inputPtr)
	SUBQ(Imm(128), t64)
	CVTSQ2SS(t64, x0)
	MOVSS(x0, Mem{Base: outputPtr})
	ADDQ(Imm(4), outputPtr)
	INCQ(index)
	CMPQ(index, maxIndex)
	JLT(LabelRef(label))
}

func u8ToF32SSE4Block(inputPtr, outputPtr, index, maxIndex Register) {
	t32 := GP32()
	x0 := XMM()
	x1 := XMM()
	toSub := XMM()

	MOVL(U32(0x80808080), t32)
	MOVD(t32, toSub)
	PSHUFL(Imm(0), toSub, toSub)

	Label("u8tof32_sse4_loop")
	Comment("Load 16 unsigned 8-bit values")
	MOVOU(Mem{Base: inputPtr}, x0)
	Comment("Make the values signed")
	PSUBB(toSub, x0)

	Comment("Lowest 4 values (bytes 0-3)")
	PMOVSXBD(x0, x1)
	Comment("Convert 32-bit signed integers to 32-bit float")
	CVTPL2PS(x1, x1)
	MOVAPS(x1, Mem{Base: outputPtr})

	Comment("Next 4 values (bytes 4-7)")
	PSHUFL(Imm(1), x0, x1)
	PMOVSXBD(x1, x1)
	Comment("Convert 32-bit signed integers to 32-bit float")
	CVTPL2PS(x1, x1)
	MOVAPS(x1, Mem{Base: outputPtr, Disp: 16})

	Comment("Next 4 values (bytes 8-11)")
	PSHUFL(Imm(2), x0, x1)
	PMOVSXBD(x1, x1)
	Comment("Convert 32-bit signed integers to 32-bit float")
	CVTPL2PS(x1, x1)
	MOVAPS(x1, Mem{Base: outputPtr, Disp: 32})

	Comment("Next 4 values (bytes 12-15)")
	PSHUFL(Imm(3), x0, x1)
	PMOVSXBD(x1, x1)
	Comment("Convert 32-bit signed integers to 32-bit float")
	CVTPL2PS(x1, x1)
	MOVAPS(x1, Mem{Base: outputPtr, Disp: 48})

	ADDQ(Imm(16), index)
	ADDQ(Imm(16), inputPtr)
	ADDQ(Imm(64), outputPtr)
	CMPQ(index, maxIndex)
	JLT(LabelRef("u8tof32_sse4_loop"))
}

func u8ToF32SSE2Block(inputPtr, outputPtr, index, maxIndex Register) {
	t32 := GP32()
	x0 := XMM()
	x1 := XMM()
	x2 := XMM()
	toSub := XMM()

	MOVL(U32(0x80808080), t32)
	MOVD(t32, toSub)
	PSHUFL(Imm(0), toSub, toSub)

	Label("u8tof32_sse2_loop")
	Comment("Load 16 unsigned 8-bit values")
	MOVOU(Mem{Base: inputPtr}, x0)
	Comment("Make the values signed")
	PSUBB(toSub, x0)
	MOVO(x0, x1)

	Comment("Lowest 4 values (bytes 0-3)")
	PUNPCKLBW(x1, x1)
	MOVO(x1, x2)
	PUNPCKLWL(x1, x1)
	PSRAL(Imm(24), x1)
	CVTPL2PS(x1, x1)
	MOVAPS(x1, Mem{Base: outputPtr})

	Comment("Next 4 values (bytes 4-7)")
	PUNPCKHWL(x2, x2)
	PSRAL(Imm(24), x2)
	CVTPL2PS(x2, x2)
	MOVAPS(x2, Mem{Base: outputPtr, Disp: 16})

	Comment("Next 4 values (bytes 8-11)")
	PUNPCKHBW(x0, x0)
	MOVO(x0, x2)
	PUNPCKLWL(x0, x0)
	PSRAL(Imm(24), x0)
	CVTPL2PS(x0, x0)
	MOVAPS(x0, Mem{Base: outputPtr, Disp: 32})

	Comment("Next 4 values (bytes 12-15)")
	PUNPCKHWL(x2, x2)
	PSRAL(Imm(24), x2)
	CVTPL2PS(x2, x2)
	MOVAPS(x2, Mem{Base: outputPtr, Disp: 48})

	ADDQ(Imm(16), index)
	ADDQ(Imm(16), inputPtr)
	ADDQ(Imm(64), outputPtr)
	CMPQ(index, maxIndex)
	JLT(LabelRef("u8tof32_sse2_loop"))
}
