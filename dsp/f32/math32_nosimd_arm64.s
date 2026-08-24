//go:build !goexperiment.simd

#include "textflag.h"

#define NegInf32 0xff800000
#define PosInf32 0x7f800000

// func Scale(dst, src []float32, scale float32)
//
// Sixty-four floats (sixteen vectors, in two halves of eight) per iteration,
// with an eight-float step behind it. FLDPQ/FSTPQ move two vectors per
// instruction and measure faster than the VLD1/VST1 multi-register forms.
//
// Sixty-four is not a free choice over thirty-two: on an M2 Pro it is worth 8%
// at 4K floats and 24% at 16K, but costs 25% at 64K, where the working set no
// longer fits L1 and the loop is bandwidth-bound. The cache-resident sizes win
// because this package processes streams in chunks. A prefetch in the loop
// recovers the 64K case for the thirty-two-float block and does nothing for
// this one, so there is none.
//
// Only V0 and V16-V23 are touched, so the question of whether Go's ABI treats
// the low halves of V8-V15 as callee-saved never arises.
TEXT ·Scale(SB), NOSPLIT, $0
	MOVD	src+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst+0(FP), R2
	MOVD	dst_len+8(FP), R3

	// Process min(len(input), len(output)) elements.
	CMP	R3, R1
	CSEL	LT, R1, R3, R1

	// Broadcast the scale to all four lanes. Going through a general register
	// avoids needing the by-element form of VDUP; F0 aliases the low lane of
	// V0, so the scalar tail below can use the same register.
	MOVW	scale+48(FP), R4
	VDUP	R4, V0.S4

	CMP	$64, R1
	BLT	vscalef32_step8

vscalef32_loop64:
	FLDPQ	(R0), (F16, F17)
	FLDPQ	32(R0), (F18, F19)
	FLDPQ	64(R0), (F20, F21)
	FLDPQ	96(R0), (F22, F23)
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	VFMUL	V0.S4, V18.S4, V18.S4
	VFMUL	V0.S4, V19.S4, V19.S4
	VFMUL	V0.S4, V20.S4, V20.S4
	VFMUL	V0.S4, V21.S4, V21.S4
	VFMUL	V0.S4, V22.S4, V22.S4
	VFMUL	V0.S4, V23.S4, V23.S4
	FSTPQ	(F16, F17), (R2)
	FSTPQ	(F18, F19), 32(R2)
	FSTPQ	(F20, F21), 64(R2)
	FSTPQ	(F22, F23), 96(R2)
	FLDPQ	128(R0), (F16, F17)
	FLDPQ	160(R0), (F18, F19)
	FLDPQ	192(R0), (F20, F21)
	FLDPQ	224(R0), (F22, F23)
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	VFMUL	V0.S4, V18.S4, V18.S4
	VFMUL	V0.S4, V19.S4, V19.S4
	VFMUL	V0.S4, V20.S4, V20.S4
	VFMUL	V0.S4, V21.S4, V21.S4
	VFMUL	V0.S4, V22.S4, V22.S4
	VFMUL	V0.S4, V23.S4, V23.S4
	FSTPQ	(F16, F17), 128(R2)
	FSTPQ	(F18, F19), 160(R2)
	FSTPQ	(F20, F21), 192(R2)
	FSTPQ	(F22, F23), 224(R2)
	ADD	$256, R0
	ADD	$256, R2
	SUB	$64, R1
	CMP	$64, R1
	BGE	vscalef32_loop64

vscalef32_step8:
	CMP	$8, R1
	BLT	vscalef32_scalar
	FLDPQ	(R0), (F16, F17)
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	FSTPQ	(F16, F17), (R2)
	ADD	$32, R0
	ADD	$32, R2
	SUB	$8, R1
	B	vscalef32_step8

vscalef32_scalar:
	CBZ	R1, vscalef32_done

vscalef32_scalar_loop:
	FMOVS.P	4(R0), F1
	FMULS	F0, F1, F1
	FMOVS.P	F1, 4(R2)
	SUBS	$1, R1
	BNE	vscalef32_scalar_loop

vscalef32_done:
	RET

// func Max(src []float32) float32
//
// Eight accumulators (V24-V31) over sixty-four floats per iteration, with
// sixteen- and four-float steps behind it. The accumulator count is what makes
// this fast: with fewer, the loop runs at FMAX latency rather than at the load
// ports. The two-accumulator version this replaces measured the same as a
// two-accumulator Go loop, and eight is 1.9x that at 1K floats.
//
// The remainder is folded in by re-reading the last full vector rather than by
// a scalar loop. Max is idempotent, so re-accumulating elements the loops
// already saw cannot change the result, and it keeps a length that is not a
// multiple of four off a serial FMAXS chain. Inputs shorter than one vector
// take the scalar path at the top instead, which also means the re-read below
// can never reach behind the start of the slice.
TEXT ·Max(SB), NOSPLIT, $0
	MOVD	src+0(FP), R0
	MOVD	src_len+8(FP), R1
	MOVW	$NegInf32, R4

	// Fewer than four elements: nothing to vectorize, and the re-read at the
	// end would underflow the slice.
	CMP	$4, R1
	BLT	vmaxf32_short

	VDUP	R4, V24.S4
	VMOV	V24.B16, V25.B16
	VMOV	V24.B16, V26.B16
	VMOV	V24.B16, V27.B16
	VMOV	V24.B16, V28.B16
	VMOV	V24.B16, V29.B16
	VMOV	V24.B16, V30.B16
	VMOV	V24.B16, V31.B16

	// Keep the base and length for the re-read of the last vector.
	MOVD	R0, R2
	MOVD	R1, R3

	CMP	$64, R1
	BLT	vmaxf32_step16

vmaxf32_loop64:
	FLDPQ	(R0), (F16, F17)
	FLDPQ	32(R0), (F18, F19)
	FLDPQ	64(R0), (F20, F21)
	FLDPQ	96(R0), (F22, F23)

	VFMAX	V16.S4, V24.S4, V24.S4
	VFMAX	V17.S4, V25.S4, V25.S4
	VFMAX	V18.S4, V26.S4, V26.S4
	VFMAX	V19.S4, V27.S4, V27.S4
	VFMAX	V20.S4, V28.S4, V28.S4
	VFMAX	V21.S4, V29.S4, V29.S4
	VFMAX	V22.S4, V30.S4, V30.S4
	VFMAX	V23.S4, V31.S4, V31.S4

	FLDPQ	128(R0), (F16, F17)
	FLDPQ	160(R0), (F18, F19)
	FLDPQ	192(R0), (F20, F21)
	FLDPQ	224(R0), (F22, F23)

	VFMAX	V16.S4, V24.S4, V24.S4
	VFMAX	V17.S4, V25.S4, V25.S4
	VFMAX	V18.S4, V26.S4, V26.S4
	VFMAX	V19.S4, V27.S4, V27.S4
	VFMAX	V20.S4, V28.S4, V28.S4
	VFMAX	V21.S4, V29.S4, V29.S4
	VFMAX	V22.S4, V30.S4, V30.S4
	VFMAX	V23.S4, V31.S4, V31.S4

	ADD	$256, R0
	SUB	$64, R1
	CMP	$64, R1
	BGE	vmaxf32_loop64

vmaxf32_step16:
	CMP	$16, R1
	BLT	vmaxf32_step4
	FLDPQ	(R0), (F16, F17)
	FLDPQ	32(R0), (F18, F19)
	VFMAX	V16.S4, V24.S4, V24.S4
	VFMAX	V17.S4, V25.S4, V25.S4
	VFMAX	V18.S4, V26.S4, V26.S4
	VFMAX	V19.S4, V27.S4, V27.S4
	ADD	$64, R0
	SUB	$16, R1
	B	vmaxf32_step16

vmaxf32_step4:
	CMP	$4, R1
	BLT	vmaxf32_remainder
	VLD1.P	16(R0), [V16.S4]
	VFMAX	V16.S4, V24.S4, V24.S4
	SUB	$4, R1
	B	vmaxf32_step4

vmaxf32_remainder:
	CBZ	R1, vmaxf32_reduce
	// Re-read the last four elements: R2 + (R3-4)*4.
	SUB	$4, R3, R3
	ADD	R3<<2, R2, R2
	VLD1	(R2), [V16.S4]
	VFMAX	V16.S4, V25.S4, V25.S4

vmaxf32_reduce:
	VFMAX	V25.S4, V24.S4, V24.S4
	VFMAX	V27.S4, V26.S4, V26.S4
	VFMAX	V29.S4, V28.S4, V28.S4
	VFMAX	V31.S4, V30.S4, V30.S4
	VFMAX	V26.S4, V24.S4, V24.S4
	VFMAX	V30.S4, V28.S4, V28.S4
	VFMAX	V28.S4, V24.S4, V24.S4
	// The assembler has no mnemonic for the across-lanes reduction.
	WORD	$0x6e30fb18	// fmaxv s24, v24.4s
	FMOVS	F24, ret+24(FP)
	RET

vmaxf32_short:
	FMOVS	R4, F0
	CBZ	R1, vmaxf32_short_done

vmaxf32_short_loop:
	FMOVS.P	4(R0), F1
	FMAXS	F0, F1, F0
	SUBS	$1, R1
	BNE	vmaxf32_short_loop

vmaxf32_short_done:
	FMOVS	F0, ret+24(FP)
	RET

// func Min(src []float32) float32
//
// The mirror of Max above — eight accumulators over sixty-four floats per
// iteration, the same sixteen- and four-float steps, and the same re-read of
// the last full vector for the remainder, which min allows for the same reason
// max does: it is idempotent. See the note there.
TEXT ·Min(SB), NOSPLIT, $0
	MOVD	src+0(FP), R0
	MOVD	src_len+8(FP), R1
	MOVW	$PosInf32, R4

	// Fewer than four elements: nothing to vectorize, and the re-read at the
	// end would underflow the slice.
	CMP	$4, R1
	BLT	vminf32_short

	VDUP	R4, V24.S4
	VMOV	V24.B16, V25.B16
	VMOV	V24.B16, V26.B16
	VMOV	V24.B16, V27.B16
	VMOV	V24.B16, V28.B16
	VMOV	V24.B16, V29.B16
	VMOV	V24.B16, V30.B16
	VMOV	V24.B16, V31.B16

	// Keep the base and length for the re-read of the last vector.
	MOVD	R0, R2
	MOVD	R1, R3

	CMP	$64, R1
	BLT	vminf32_step16

vminf32_loop64:
	FLDPQ	(R0), (F16, F17)
	FLDPQ	32(R0), (F18, F19)
	FLDPQ	64(R0), (F20, F21)
	FLDPQ	96(R0), (F22, F23)

	VFMIN	V16.S4, V24.S4, V24.S4
	VFMIN	V17.S4, V25.S4, V25.S4
	VFMIN	V18.S4, V26.S4, V26.S4
	VFMIN	V19.S4, V27.S4, V27.S4
	VFMIN	V20.S4, V28.S4, V28.S4
	VFMIN	V21.S4, V29.S4, V29.S4
	VFMIN	V22.S4, V30.S4, V30.S4
	VFMIN	V23.S4, V31.S4, V31.S4

	FLDPQ	128(R0), (F16, F17)
	FLDPQ	160(R0), (F18, F19)
	FLDPQ	192(R0), (F20, F21)
	FLDPQ	224(R0), (F22, F23)

	VFMIN	V16.S4, V24.S4, V24.S4
	VFMIN	V17.S4, V25.S4, V25.S4
	VFMIN	V18.S4, V26.S4, V26.S4
	VFMIN	V19.S4, V27.S4, V27.S4
	VFMIN	V20.S4, V28.S4, V28.S4
	VFMIN	V21.S4, V29.S4, V29.S4
	VFMIN	V22.S4, V30.S4, V30.S4
	VFMIN	V23.S4, V31.S4, V31.S4

	ADD	$256, R0
	SUB	$64, R1
	CMP	$64, R1
	BGE	vminf32_loop64

vminf32_step16:
	CMP	$16, R1
	BLT	vminf32_step4
	FLDPQ	(R0), (F16, F17)
	FLDPQ	32(R0), (F18, F19)
	VFMIN	V16.S4, V24.S4, V24.S4
	VFMIN	V17.S4, V25.S4, V25.S4
	VFMIN	V18.S4, V26.S4, V26.S4
	VFMIN	V19.S4, V27.S4, V27.S4
	ADD	$64, R0
	SUB	$16, R1
	B	vminf32_step16

vminf32_step4:
	CMP	$4, R1
	BLT	vminf32_remainder
	VLD1.P	16(R0), [V16.S4]
	VFMIN	V16.S4, V24.S4, V24.S4
	SUB	$4, R1
	B	vminf32_step4

vminf32_remainder:
	CBZ	R1, vminf32_reduce
	// Re-read the last four elements: R2 + (R3-4)*4.
	SUB	$4, R3, R3
	ADD	R3<<2, R2, R2
	VLD1	(R2), [V16.S4]
	VFMIN	V16.S4, V25.S4, V25.S4

vminf32_reduce:
	VFMIN	V25.S4, V24.S4, V24.S4
	VFMIN	V27.S4, V26.S4, V26.S4
	VFMIN	V29.S4, V28.S4, V28.S4
	VFMIN	V31.S4, V30.S4, V30.S4
	VFMIN	V26.S4, V24.S4, V24.S4
	VFMIN	V30.S4, V28.S4, V28.S4
	VFMIN	V28.S4, V24.S4, V24.S4
	// The assembler has no mnemonic for the across-lanes reduction.
	WORD	$0x6eb0fb18	// fminv s24, v24.4s
	FMOVS	F24, ret+24(FP)
	RET

vminf32_short:
	FMOVS	R4, F0
	CBZ	R1, vminf32_short_done

vminf32_short_loop:
	FMOVS.P	4(R0), F1
	FMINS	F0, F1, F0
	SUBS	$1, R1
	BNE	vminf32_short_loop

vminf32_short_done:
	FMOVS	F0, ret+24(FP)
	RET

// func CAbs(dst []float32, src []complex64)
//
// VLD2 deinterleaves as it loads — one instruction produces a vector of four
// real parts and a vector of four imaginary parts — so unlike amd64 there is no
// shuffle to arrange. Sixteen magnitudes (four output vectors) per iteration,
// with a four-magnitude step and a scalar tail behind it.
//
// The squares are rounded separately and then added, rather than folded with
// VFMLA. That is not an oversight: it is the behavior the scalar reference
// specifies and the only one every target can produce. See the note on
// vabsC64Scalar.
//
// Sixteen magnitudes per iteration is the block the archsimd version uses, where
// a sweep put the knee: below four output vectors the loop gives up 4-16%
// depending on length, and above four it gains nothing.
TEXT ·CAbs(SB), NOSPLIT, $0
	MOVD	src+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst+0(FP), R2
	MOVD	dst_len+8(FP), R3

	// Process min(len(input), len(output)) complex values.
	CMP	R3, R1
	CSEL	LT, R1, R3, R1

	CMP	$16, R1
	BLT	vabsc64_step4

vabsc64_loop16:
	VLD2.P	32(R0), [V0.S4, V1.S4]
	VLD2.P	32(R0), [V2.S4, V3.S4]
	VLD2.P	32(R0), [V4.S4, V5.S4]
	VLD2.P	32(R0), [V6.S4, V7.S4]

	VFMUL	V0.S4, V0.S4, V16.S4
	VFMUL	V1.S4, V1.S4, V17.S4
	VFMUL	V2.S4, V2.S4, V18.S4
	VFMUL	V3.S4, V3.S4, V19.S4
	VFMUL	V4.S4, V4.S4, V20.S4
	VFMUL	V5.S4, V5.S4, V21.S4
	VFMUL	V6.S4, V6.S4, V22.S4
	VFMUL	V7.S4, V7.S4, V23.S4

	VFADD	V17.S4, V16.S4, V16.S4
	VFADD	V19.S4, V18.S4, V18.S4
	VFADD	V21.S4, V20.S4, V20.S4
	VFADD	V23.S4, V22.S4, V22.S4

	VFSQRT	V16.S4, V16.S4
	VFSQRT	V18.S4, V18.S4
	VFSQRT	V20.S4, V20.S4
	VFSQRT	V22.S4, V22.S4

	VST1.P	[V16.S4], 16(R2)
	VST1.P	[V18.S4], 16(R2)
	VST1.P	[V20.S4], 16(R2)
	VST1.P	[V22.S4], 16(R2)

	SUB	$16, R1
	CMP	$16, R1
	BGE	vabsc64_loop16

vabsc64_step4:
	CMP	$4, R1
	BLT	vabsc64_scalar
	VLD2.P	32(R0), [V0.S4, V1.S4]
	VFMUL	V0.S4, V0.S4, V16.S4
	VFMUL	V1.S4, V1.S4, V17.S4
	VFADD	V17.S4, V16.S4, V16.S4
	VFSQRT	V16.S4, V16.S4
	VST1.P	[V16.S4], 16(R2)
	SUB	$4, R1
	B	vabsc64_step4

vabsc64_scalar:
	CBZ	R1, vabsc64_done

vabsc64_scalar_loop:
	FMOVS.P	4(R0), F0
	FMOVS.P	4(R0), F1
	FMULS	F0, F0, F0
	FMULS	F1, F1, F1
	FADDS	F1, F0, F0
	FSQRTS	F0, F0
	FMOVS.P	F0, 4(R2)
	SUBS	$1, R1
	BNE	vabsc64_scalar_loop

vabsc64_done:
	RET
