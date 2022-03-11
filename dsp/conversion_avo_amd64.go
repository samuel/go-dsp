//go:build ignore
// +build ignore

package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	. "github.com/mmcloughlin/avo/reg"
)

func main() {
	TEXT("Ui8tof32", NOSPLIT, "func(input []byte, output []float32)")
	Doc("Ui8tof32 converts unsigned 8-bit samples to 32-bit float.")
	inputPtr := Load(Param("input").Base(), GP64())
	inputLen := Load(Param("input").Len(), GP64())
	outputPtr := Load(Param("output").Base(), GP64())
	outputLen := Load(Param("output").Len(), GP64())

	Comment("Pick shortest length")
	CMPQ(outputLen, inputLen)
	JGE(LabelRef("ui8tof32_min_len"))
	MOVQ(outputLen, inputLen)
	Label("ui8tof32_min_len")

	index := GP64()
	MOVQ(U64(0), index)

	t64 := GP64()

	Comment("If input is too short to optimize (less than 32 bytes) then single step")
	MOVQ(U64(32), t64)
	CMPQ(t64, inputLen)
	JGE(LabelRef("ui8tof32_stepper"))

	Comment("Align output to 16-byte boundary")
	MOVQ(outputPtr, t64)
	ANDQ(Imm(0xf), t64)
	SHRQ(Imm(2), t64) // divide by 4 to convert bytes to 32-bit blocks
	JZ(LabelRef("ui8tof32_aligned"))

	t2 := GP64()
	MOVQ(U64(4), t2)
	SUBQ(t64, t2)
	ui8tof32Step(inputPtr, outputPtr, index, t2, "ui8tof32_align")

	Label("ui8tof32_aligned")
	n := GP64()
	MOVQ(inputLen, n)
	ANDQ(U32(^uint32(15)), n)
	CMPQ(index, n)
	JGE(LabelRef("ui8tof32_stepper"))

	// CMPB(NewDataAddr(Symbol{Name: "·x86+const_offsetX86HasSSE41"}, 0), Imm(1))
	CMPB(NewDataAddr(Symbol{Name: "·useSSE4"}, 0), Imm(1))
	JNE(LabelRef("ui8tof32_nosse4"))

	ui8tof32SSE4(inputPtr, outputPtr, index, n)

	JMP(LabelRef("ui8tof32_stepper"))

	Label("ui8tof32_nosse4")

	ui8tof32SSE2(inputPtr, outputPtr, index, n)

	Comment("TODO: work increasingly smaller blocks")

	Label("ui8tof32_stepper")
	CMPQ(index, inputLen)
	JGE(LabelRef("ui8tof32_done"))

	ui8tof32Step(inputPtr, outputPtr, index, inputLen, "ui8tof32_step")

	Label("ui8tof32_done")
	RET()

	Generate()
}

func ui8tof32Step(inputPtr, outputPtr, index, maxIndex Register, label string) {
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

func ui8tof32SSE4(inputPtr, outputPtr, index, maxIndex Register) {
	t32 := GP32()
	x0 := XMM()
	x1 := XMM()
	toSub := XMM()

	MOVL(U32(0x80808080), t32)
	MOVD(t32, toSub)
	PSHUFL(Imm(0), toSub, toSub)

	Label("ui8tof32_sse4_loop")
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
	JLT(LabelRef("ui8tof32_sse4_loop"))
}

func ui8tof32SSE2(inputPtr, outputPtr, index, maxIndex Register) {
	t32 := GP32()
	x0 := XMM()
	x1 := XMM()
	x2 := XMM()
	toSub := XMM()

	MOVL(U32(0x80808080), t32)
	MOVD(t32, toSub)
	PSHUFL(Imm(0), toSub, toSub)

	Label("ui8tof32_sse2_loop")
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
	JLT(LabelRef("ui8tof32_sse2_loop"))
}
