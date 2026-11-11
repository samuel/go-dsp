//go:build !goexperiment.simd

#include "textflag.h"

// Every kernel here multiplies with VFMUL and adds with VFADD, never VFMLA: the
// references round each product before adding it, and a fused multiply-add
// would not. Only V0-V7 and V16-V31 are touched, as in math32_nosimd_arm64.s.

// func firNEON(dst, work, taps []float32, step int)
//
// dst[k] = sum over j of taps[j]*work[k+(len(taps)-1-j)*step], for every k
// below len(dst). step is 1 for FIR and 2 for CFIRReal, which runs this over
// the interleaved floats of its complex slices, so that the taps reach back one
// complex sample at a time.
//
// The loop is across outputs: lane l of a block accumulates output k+l over the
// taps in order, which is the reference's order for any block size. A block of
// thirty-two outputs keeps eight accumulators in flight, enough to hide the
// FADD latency; eight- and four-output blocks and a scalar loop finish the rest.
//
// R0 dst, R1 outputs left, R2 work at the block's first output, R3 taps,
// R4 len(taps), R5 step in bytes, R6 (len(taps)-1)*step in bytes; R7 walks
// back through work and R8 forward through taps for each block, R9 counts taps.
TEXT ·firNEON(SB), NOSPLIT, $0-80
	MOVD	dst+0(FP), R0
	MOVD	dst_len+8(FP), R1
	MOVD	work+24(FP), R2
	MOVD	taps+48(FP), R3
	MOVD	taps_len+56(FP), R4
	MOVD	step+72(FP), R5
	LSL	$2, R5, R5
	SUB	$1, R4, R6
	MUL	R5, R6, R6

fir_block32:
	CMP	$32, R1
	BLT	fir_block8
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	VEOR	V26.B16, V26.B16, V26.B16
	VEOR	V27.B16, V27.B16, V27.B16
	VEOR	V28.B16, V28.B16, V28.B16
	VEOR	V29.B16, V29.B16, V29.B16
	VEOR	V30.B16, V30.B16, V30.B16
	VEOR	V31.B16, V31.B16, V31.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

fir_tap32:
	VLD1R.P	4(R8), [V0.S4]
	FLDPQ	(R7), (F16, F17)
	FLDPQ	32(R7), (F18, F19)
	FLDPQ	64(R7), (F20, F21)
	FLDPQ	96(R7), (F22, F23)
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	VFMUL	V0.S4, V18.S4, V18.S4
	VFMUL	V0.S4, V19.S4, V19.S4
	VFMUL	V0.S4, V20.S4, V20.S4
	VFMUL	V0.S4, V21.S4, V21.S4
	VFMUL	V0.S4, V22.S4, V22.S4
	VFMUL	V0.S4, V23.S4, V23.S4
	VFADD	V16.S4, V24.S4, V24.S4
	VFADD	V17.S4, V25.S4, V25.S4
	VFADD	V18.S4, V26.S4, V26.S4
	VFADD	V19.S4, V27.S4, V27.S4
	VFADD	V20.S4, V28.S4, V28.S4
	VFADD	V21.S4, V29.S4, V29.S4
	VFADD	V22.S4, V30.S4, V30.S4
	VFADD	V23.S4, V31.S4, V31.S4
	SUB	R5, R7, R7
	SUBS	$1, R9, R9
	BNE	fir_tap32

	FSTPQ	(F24, F25), (R0)
	FSTPQ	(F26, F27), 32(R0)
	FSTPQ	(F28, F29), 64(R0)
	FSTPQ	(F30, F31), 96(R0)
	ADD	$128, R0
	ADD	$128, R2
	SUB	$32, R1
	B	fir_block32

fir_block8:
	CMP	$8, R1
	BLT	fir_block4
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

fir_tap8:
	VLD1R.P	4(R8), [V0.S4]
	FLDPQ	(R7), (F16, F17)
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	VFADD	V16.S4, V24.S4, V24.S4
	VFADD	V17.S4, V25.S4, V25.S4
	SUB	R5, R7, R7
	SUBS	$1, R9, R9
	BNE	fir_tap8

	FSTPQ	(F24, F25), (R0)
	ADD	$32, R0
	ADD	$32, R2
	SUB	$8, R1
	B	fir_block8

fir_block4:
	CMP	$4, R1
	BLT	fir_scalar
	VEOR	V24.B16, V24.B16, V24.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

fir_tap4:
	VLD1R.P	4(R8), [V0.S4]
	VLD1	(R7), [V16.S4]
	VFMUL	V0.S4, V16.S4, V16.S4
	VFADD	V16.S4, V24.S4, V24.S4
	SUB	R5, R7, R7
	SUBS	$1, R9, R9
	BNE	fir_tap4

	VST1.P	[V24.S4], 16(R0)
	ADD	$16, R2
	SUB	$4, R1

fir_scalar:
	CBZ	R1, fir_done

fir_scalar_loop:
	VEOR	V24.B16, V24.B16, V24.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

fir_scalar_tap:
	FMOVS.P	4(R8), F0
	FMOVS	(R7), F16
	FMULS	F0, F16, F16
	FADDS	F16, F24, F24
	SUB	R5, R7, R7
	SUBS	$1, R9, R9
	BNE	fir_scalar_tap

	FMOVS.P	F24, 4(R0)
	ADD	$4, R2
	SUBS	$1, R1, R1
	BNE	fir_scalar_loop

fir_done:
	RET

// func cfirNEON(dst, work, taps []complex64)
//
// FIR with complex taps, across outputs as firNEON, on the interleaved floats.
// For a vector v of two samples and a tap (cr, ci), the term is
//
//	v*(cr, cr) + rev64(v)*(-ci, ci)
//
// lane by lane: (cr*vr - ci*vi, cr*vi + ci*vr), with each product rounded and
// the negation exact, so adding the negated product is the reference's
// subtraction bit for bit. VLD2R broadcasts cr and ci and V7 flips the sign
// of ci's real lanes.
//
// Splitting the parts with VLD2 instead saves the REV64s but measured 3%
// slower on an M2 Pro: both forms are bound by vector issue, and the
// deinterleaving load costs as much as the shuffles it replaces.
//
// Sixteen outputs (eight accumulators) per block, then four, then two, then a
// scalar step. R0-R4, R6-R9 as in firNEON, with work stepping back eight bytes
// per tap.
TEXT ·cfirNEON(SB), NOSPLIT, $0-72
	MOVD	dst+0(FP), R0
	MOVD	dst_len+8(FP), R1
	MOVD	work+24(FP), R2
	MOVD	taps+48(FP), R3
	MOVD	taps_len+56(FP), R4
	SUB	$1, R4, R6
	LSL	$3, R6, R6
	MOVD	$0x80000000, R10
	VDUP	R10, V7.D2

cfir_block16:
	CMP	$16, R1
	BLT	cfir_block4
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	VEOR	V26.B16, V26.B16, V26.B16
	VEOR	V27.B16, V27.B16, V27.B16
	VEOR	V28.B16, V28.B16, V28.B16
	VEOR	V29.B16, V29.B16, V29.B16
	VEOR	V30.B16, V30.B16, V30.B16
	VEOR	V31.B16, V31.B16, V31.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

cfir_tap16:
	VLD2R.P	8(R8), [V0.S4, V1.S4]
	VEOR	V7.B16, V1.B16, V1.B16
	FLDPQ	(R7), (F16, F17)
	FLDPQ	32(R7), (F18, F19)
	FLDPQ	64(R7), (F20, F21)
	FLDPQ	96(R7), (F22, F23)
	VREV64	V16.S4, V2.S4
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V1.S4, V2.S4, V2.S4
	VFADD	V2.S4, V16.S4, V16.S4
	VFADD	V16.S4, V24.S4, V24.S4
	VREV64	V17.S4, V3.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	VFMUL	V1.S4, V3.S4, V3.S4
	VFADD	V3.S4, V17.S4, V17.S4
	VFADD	V17.S4, V25.S4, V25.S4
	VREV64	V18.S4, V4.S4
	VFMUL	V0.S4, V18.S4, V18.S4
	VFMUL	V1.S4, V4.S4, V4.S4
	VFADD	V4.S4, V18.S4, V18.S4
	VFADD	V18.S4, V26.S4, V26.S4
	VREV64	V19.S4, V5.S4
	VFMUL	V0.S4, V19.S4, V19.S4
	VFMUL	V1.S4, V5.S4, V5.S4
	VFADD	V5.S4, V19.S4, V19.S4
	VFADD	V19.S4, V27.S4, V27.S4
	VREV64	V20.S4, V6.S4
	VFMUL	V0.S4, V20.S4, V20.S4
	VFMUL	V1.S4, V6.S4, V6.S4
	VFADD	V6.S4, V20.S4, V20.S4
	VFADD	V20.S4, V28.S4, V28.S4
	VREV64	V21.S4, V2.S4
	VFMUL	V0.S4, V21.S4, V21.S4
	VFMUL	V1.S4, V2.S4, V2.S4
	VFADD	V2.S4, V21.S4, V21.S4
	VFADD	V21.S4, V29.S4, V29.S4
	VREV64	V22.S4, V3.S4
	VFMUL	V0.S4, V22.S4, V22.S4
	VFMUL	V1.S4, V3.S4, V3.S4
	VFADD	V3.S4, V22.S4, V22.S4
	VFADD	V22.S4, V30.S4, V30.S4
	VREV64	V23.S4, V4.S4
	VFMUL	V0.S4, V23.S4, V23.S4
	VFMUL	V1.S4, V4.S4, V4.S4
	VFADD	V4.S4, V23.S4, V23.S4
	VFADD	V23.S4, V31.S4, V31.S4
	SUB	$8, R7, R7
	SUBS	$1, R9, R9
	BNE	cfir_tap16

	FSTPQ	(F24, F25), (R0)
	FSTPQ	(F26, F27), 32(R0)
	FSTPQ	(F28, F29), 64(R0)
	FSTPQ	(F30, F31), 96(R0)
	ADD	$128, R0
	ADD	$128, R2
	SUB	$16, R1
	B	cfir_block16

cfir_block4:
	CMP	$4, R1
	BLT	cfir_block2
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

cfir_tap4:
	VLD2R.P	8(R8), [V0.S4, V1.S4]
	VEOR	V7.B16, V1.B16, V1.B16
	FLDPQ	(R7), (F16, F17)
	VREV64	V16.S4, V2.S4
	VREV64	V17.S4, V3.S4
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V0.S4, V17.S4, V17.S4
	VFMUL	V1.S4, V2.S4, V2.S4
	VFMUL	V1.S4, V3.S4, V3.S4
	VFADD	V2.S4, V16.S4, V16.S4
	VFADD	V3.S4, V17.S4, V17.S4
	VFADD	V16.S4, V24.S4, V24.S4
	VFADD	V17.S4, V25.S4, V25.S4
	SUB	$8, R7, R7
	SUBS	$1, R9, R9
	BNE	cfir_tap4

	FSTPQ	(F24, F25), (R0)
	ADD	$32, R0
	ADD	$32, R2
	SUB	$4, R1
	B	cfir_block4

cfir_block2:
	CMP	$2, R1
	BLT	cfir_scalar
	VEOR	V24.B16, V24.B16, V24.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

cfir_tap2:
	VLD2R.P	8(R8), [V0.S4, V1.S4]
	VEOR	V7.B16, V1.B16, V1.B16
	VLD1	(R7), [V16.S4]
	VREV64	V16.S4, V2.S4
	VFMUL	V0.S4, V16.S4, V16.S4
	VFMUL	V1.S4, V2.S4, V2.S4
	VFADD	V2.S4, V16.S4, V16.S4
	VFADD	V16.S4, V24.S4, V24.S4
	SUB	$8, R7, R7
	SUBS	$1, R9, R9
	BNE	cfir_tap2

	VST1.P	[V24.S4], 16(R0)
	ADD	$16, R2
	SUB	$2, R1

cfir_scalar:
	CBZ	R1, cfir_done

cfir_scalar_loop:
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	ADD	R6, R2, R7
	MOVD	R3, R8
	MOVD	R4, R9

cfir_scalar_tap:
	FLDPS.P	8(R8), (F0, F1)
	FLDPS	(R7), (F16, F17)
	FMULS	F0, F16, F2
	FMULS	F1, F17, F3
	FMULS	F0, F17, F17
	FMULS	F1, F16, F16
	FSUBS	F3, F2, F2
	FADDS	F16, F17, F17
	FADDS	F2, F24, F24
	FADDS	F17, F25, F25
	SUB	$8, R7, R7
	SUBS	$1, R9, R9
	BNE	cfir_scalar_tap

	FSTPS.P	(F24, F25), 8(R0)
	ADD	$8, R2
	SUBS	$1, R1, R1
	BNE	cfir_scalar_loop

cfir_done:
	RET

// func dotNEON(x, c []float32) float32
//
// The eight accumulators of dotF32Go are the lanes of V24 (0-3) and V25 (4-7).
// combine8 is then V24+V25, plus that sum with its halves swapped, and the two
// low lanes added by FADDP; the remainder is added after, in order.
//
// The lanes are serial chains, so a longer block would change the result; two
// independent FADDs per eight elements is what this order allows.
TEXT ·dotNEON(SB), NOSPLIT, $0-52
	MOVD	x+0(FP), R0
	MOVD	c+24(FP), R2
	MOVD	c_len+32(FP), R1
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	LSR	$3, R1, R3
	CBZ	R3, dot_combine

dot_loop8:
	FLDPQ.P	32(R0), (F16, F17)
	FLDPQ.P	32(R2), (F18, F19)
	VFMUL	V18.S4, V16.S4, V16.S4
	VFMUL	V19.S4, V17.S4, V17.S4
	VFADD	V16.S4, V24.S4, V24.S4
	VFADD	V17.S4, V25.S4, V25.S4
	SUBS	$1, R3, R3
	BNE	dot_loop8

dot_combine:
	VFADD	V25.S4, V24.S4, V24.S4
	VEXT	$8, V24.B16, V24.B16, V25.B16
	VFADD	V25.S4, V24.S4, V24.S4
	VFADDP	V24.S2, V24.S2, V24.S2

	ANDS	$7, R1, R1
	BEQ	dot_done

dot_tail:
	FMOVS.P	4(R0), F16
	FMOVS.P	4(R2), F18
	FMULS	F18, F16, F16
	FADDS	F16, F24, F24
	SUBS	$1, R1, R1
	BNE	dot_tail

dot_done:
	FMOVS	F24, ret+48(FP)
	RET

// func cdotRealNEON(x []complex64, c []float32) complex64
//
// dotNEON over each part: VLD2 splits eight complex samples into the real
// parts (V16, V18) and imaginary parts (V17, V19), and the real accumulators
// V24/V25 and imaginary V26/V27 are combined exactly as dotNEON combines its
// pair, so each part is Dot of that part bit for bit.
TEXT ·cdotRealNEON(SB), NOSPLIT, $0-56
	MOVD	x+0(FP), R0
	MOVD	c+24(FP), R2
	MOVD	c_len+32(FP), R1
	VEOR	V24.B16, V24.B16, V24.B16
	VEOR	V25.B16, V25.B16, V25.B16
	VEOR	V26.B16, V26.B16, V26.B16
	VEOR	V27.B16, V27.B16, V27.B16
	LSR	$3, R1, R3
	CBZ	R3, cdot_combine

cdot_loop8:
	VLD2.P	32(R0), [V16.S4, V17.S4]
	VLD2.P	32(R0), [V18.S4, V19.S4]
	FLDPQ.P	32(R2), (F20, F21)
	VFMUL	V20.S4, V16.S4, V16.S4
	VFMUL	V20.S4, V17.S4, V17.S4
	VFMUL	V21.S4, V18.S4, V18.S4
	VFMUL	V21.S4, V19.S4, V19.S4
	VFADD	V16.S4, V24.S4, V24.S4
	VFADD	V17.S4, V26.S4, V26.S4
	VFADD	V18.S4, V25.S4, V25.S4
	VFADD	V19.S4, V27.S4, V27.S4
	SUBS	$1, R3, R3
	BNE	cdot_loop8

cdot_combine:
	VFADD	V25.S4, V24.S4, V24.S4
	VFADD	V27.S4, V26.S4, V26.S4
	VEXT	$8, V24.B16, V24.B16, V25.B16
	VEXT	$8, V26.B16, V26.B16, V27.B16
	VFADD	V25.S4, V24.S4, V24.S4
	VFADD	V27.S4, V26.S4, V26.S4
	VFADDP	V24.S2, V24.S2, V24.S2
	VFADDP	V26.S2, V26.S2, V26.S2

	ANDS	$7, R1, R1
	BEQ	cdot_done

cdot_tail:
	FLDPS.P	8(R0), (F16, F17)
	FMOVS.P	4(R2), F18
	FMULS	F18, F16, F16
	FMULS	F18, F17, F17
	FADDS	F16, F24, F24
	FADDS	F17, F26, F26
	SUBS	$1, R1, R1
	BNE	cdot_tail

cdot_done:
	FMOVS	F24, ret_real+48(FP)
	FMOVS	F26, ret_imag+52(FP)
	RET
