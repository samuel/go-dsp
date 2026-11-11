// NEON kernels for the stride-1 FIR filters and the dot products in fir.go,
// whose references define every result. Products are rounded by VMUL before
// VADD adds them, never fused, so each lane repeats the reference's
// arithmetic exactly. The scalar tails do the same with VFP MULF and ADDF.
//
// The FIR kernels vectorize across outputs: lane l of a block accumulates
// output k+l over the taps in order, from a load at work[k+l+T-1-j] times the
// broadcast tap j.

#include "textflag.h"

// func firF32NEON(dst, work, taps []float32)
TEXT ·firF32NEON(SB), NOSPLIT, $0-36
	MOVW dst_base+0(FP), R0
	MOVW dst_len+4(FP), R1
	MOVW work_base+12(FP), R2
	MOVW taps_base+24(FP), R3
	MOVW taps_len+28(FP), R4

	CMP $0, R1
	BEQ fir_done

	// R9 is the byte offset of the newest sample an output reads.
	SUB $1, R4, R9
	MOVW R9<<2, R9

	CMP $16, R1
	BLT fir_4

	// Sixteen outputs at a time in q8-q11.
fir_16:
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVW $-36, R8
	WORD $0xf2c00050 // vmov.i32 q8, #0
	WORD $0xf2c02050 // vmov.i32 q9, #0
	WORD $0xf2c04050 // vmov.i32 q10, #0
	WORD $0xf2c06050 // vmov.i32 q11, #0

fir_16_tap:
	MOVF  (R6), F0
	ADD   $4, R6
	WORD  $0xf465828d // vld1.32 {d24-d27}, [r5]!
	WORD  $0xf465c288 // vld1.32 {d28-d31}, [r5], r8
	WORD  $0xf3e889c0 // vmul.f32 q12, q12, d0[0]
	WORD  $0xf3eaa9c0 // vmul.f32 q13, q13, d0[0]
	WORD  $0xf3ecc9c0 // vmul.f32 q14, q14, d0[0]
	WORD  $0xf3eee9c0 // vmul.f32 q15, q15, d0[0]
	WORD  $0xf2400de8 // vadd.f32 q8, q8, q12
	WORD  $0xf2422dea // vadd.f32 q9, q9, q13
	WORD  $0xf2444dec // vadd.f32 q10, q10, q14
	WORD  $0xf2466dee // vadd.f32 q11, q11, q15
	SUB.S $1, R7
	BNE   fir_16_tap

	WORD $0xf440028d // vst1.32 {d16-d19}, [r0]!
	WORD $0xf440428d // vst1.32 {d20-d23}, [r0]!
	ADD  $64, R2
	SUB  $16, R1
	CMP  $16, R1
	BGE  fir_16

	// Four outputs at a time in q8.
fir_4:
	CMP  $4, R1
	BLT  fir_1
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVW $-4, R8
	WORD $0xf2c00050 // vmov.i32 q8, #0

fir_4_tap:
	MOVF  (R6), F0
	ADD   $4, R6
	WORD  $0xf4658a88 // vld1.32 {d24-d25}, [r5], r8
	WORD  $0xf3e889c0 // vmul.f32 q12, q12, d0[0]
	WORD  $0xf2400de8 // vadd.f32 q8, q8, q12
	SUB.S $1, R7
	BNE   fir_4_tap

	WORD $0xf4400a8d // vst1.32 {d16-d17}, [r0]!
	ADD  $16, R2
	SUB  $4, R1
	B    fir_4

	// The last few outputs one at a time.
fir_1:
	CMP  $0, R1
	BEQ  fir_done
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVF $0.0, F1

fir_1_tap:
	MOVF  (R6), F0
	MOVF  (R5), F2
	ADD   $4, R6
	SUB   $4, R5
	MULF  F0, F2
	ADDF  F2, F1
	SUB.S $1, R7
	BNE   fir_1_tap

	MOVF F1, (R0)
	ADD  $4, R0
	ADD  $4, R2
	SUB  $1, R1
	B    fir_1

fir_done:
	RET

// func cfirRealNEON(dst, work []complex64, taps []float32)
//
// FIR over the interleaved floats, with each tap stepping back one complex
// sample: a block of sixteen floats is eight outputs.
TEXT ·cfirRealNEON(SB), NOSPLIT, $0-36
	MOVW dst_base+0(FP), R0
	MOVW dst_len+4(FP), R1
	MOVW work_base+12(FP), R2
	MOVW taps_base+24(FP), R3
	MOVW taps_len+28(FP), R4

	CMP $0, R1
	BEQ cfirreal_done

	SUB $1, R4, R9
	MOVW R9<<3, R9

	CMP $8, R1
	BLT cfirreal_2

cfirreal_8:
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVW $-40, R8
	WORD $0xf2c00050 // vmov.i32 q8, #0
	WORD $0xf2c02050 // vmov.i32 q9, #0
	WORD $0xf2c04050 // vmov.i32 q10, #0
	WORD $0xf2c06050 // vmov.i32 q11, #0

cfirreal_8_tap:
	MOVF  (R6), F0
	ADD   $4, R6
	WORD  $0xf465828d // vld1.32 {d24-d27}, [r5]!
	WORD  $0xf465c288 // vld1.32 {d28-d31}, [r5], r8
	WORD  $0xf3e889c0 // vmul.f32 q12, q12, d0[0]
	WORD  $0xf3eaa9c0 // vmul.f32 q13, q13, d0[0]
	WORD  $0xf3ecc9c0 // vmul.f32 q14, q14, d0[0]
	WORD  $0xf3eee9c0 // vmul.f32 q15, q15, d0[0]
	WORD  $0xf2400de8 // vadd.f32 q8, q8, q12
	WORD  $0xf2422dea // vadd.f32 q9, q9, q13
	WORD  $0xf2444dec // vadd.f32 q10, q10, q14
	WORD  $0xf2466dee // vadd.f32 q11, q11, q15
	SUB.S $1, R7
	BNE   cfirreal_8_tap

	WORD $0xf440028d // vst1.32 {d16-d19}, [r0]!
	WORD $0xf440428d // vst1.32 {d20-d23}, [r0]!
	ADD  $64, R2
	SUB  $8, R1
	CMP  $8, R1
	BGE  cfirreal_8

cfirreal_2:
	CMP  $2, R1
	BLT  cfirreal_1
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVW $-8, R8
	WORD $0xf2c00050 // vmov.i32 q8, #0

cfirreal_2_tap:
	MOVF  (R6), F0
	ADD   $4, R6
	WORD  $0xf4658a88 // vld1.32 {d24-d25}, [r5], r8
	WORD  $0xf3e889c0 // vmul.f32 q12, q12, d0[0]
	WORD  $0xf2400de8 // vadd.f32 q8, q8, q12
	SUB.S $1, R7
	BNE   cfirreal_2_tap

	WORD $0xf4400a8d // vst1.32 {d16-d17}, [r0]!
	ADD  $16, R2
	SUB  $2, R1
	B    cfirreal_2

	// At most one output is left.
cfirreal_1:
	CMP  $0, R1
	BEQ  cfirreal_done
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVF $0.0, F1
	MOVF $0.0, F3

cfirreal_1_tap:
	MOVF  (R6), F0
	MOVF  0(R5), F2
	MOVF  4(R5), F4
	ADD   $4, R6
	SUB   $8, R5
	MULF  F0, F2
	MULF  F0, F4
	ADDF  F2, F1
	ADDF  F4, F3
	SUB.S $1, R7
	BNE   cfirreal_1_tap

	MOVF F1, 0(R0)
	MOVF F3, 4(R0)

cfirreal_done:
	RET

// func cfirNEON(dst, work, taps []complex64)
//
// For a tap (cr, ci) and samples v = (vr0, vi0, vr1, vi1), the term is
// v*cr + rev64(v)*(-ci, ci, -ci, ci): the real lanes add round(cr*vr) and
// round(-ci*vi), which is the reference's round(cr*vr) - round(ci*vi), and
// the imaginary lanes add round(cr*vi) and round(ci*vr). q2 holds the mask
// that negates the real lanes of the broadcast ci.
TEXT ·cfirNEON(SB), NOSPLIT, $0-36
	MOVW dst_base+0(FP), R0
	MOVW dst_len+4(FP), R1
	MOVW work_base+12(FP), R2
	MOVW taps_base+24(FP), R3
	MOVW taps_len+28(FP), R4

	CMP $0, R1
	BEQ cfir_done

	SUB $1, R4, R9
	MOVW R9<<3, R9

	MOVW $0x80000000, R7
	MOVW $0, R8
	WORD $0xec487b14 // vmov d4, r7, r8
	WORD $0xec487b15 // vmov d5, r7, r8

	CMP $8, R1
	BLT cfir_2

cfir_8:
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVW $-40, R8
	WORD $0xf2c00050 // vmov.i32 q8, #0
	WORD $0xf2c02050 // vmov.i32 q9, #0
	WORD $0xf2c04050 // vmov.i32 q10, #0
	WORD $0xf2c06050 // vmov.i32 q11, #0

cfir_8_tap:
	MOVD  (R6), F0
	ADD   $8, R6
	WORD  $0xf3bc2c40 // vdup.32 q1, d0[1]
	WORD  $0xf3022154 // veor q1, q1, q2
	WORD  $0xf465828d // vld1.32 {d24-d27}, [r5]!
	WORD  $0xf465c288 // vld1.32 {d28-d31}, [r5], r8
	WORD  $0xf3b86068 // vrev64.32 q3, q12
	WORD  $0xf3b8806a // vrev64.32 q4, q13
	WORD  $0xf3b8a06c // vrev64.32 q5, q14
	WORD  $0xf3b8c06e // vrev64.32 q6, q15
	WORD  $0xf3e889c0 // vmul.f32 q12, q12, d0[0]
	WORD  $0xf3eaa9c0 // vmul.f32 q13, q13, d0[0]
	WORD  $0xf3ecc9c0 // vmul.f32 q14, q14, d0[0]
	WORD  $0xf3eee9c0 // vmul.f32 q15, q15, d0[0]
	WORD  $0xf3066d52 // vmul.f32 q3, q3, q1
	WORD  $0xf3088d52 // vmul.f32 q4, q4, q1
	WORD  $0xf30aad52 // vmul.f32 q5, q5, q1
	WORD  $0xf30ccd52 // vmul.f32 q6, q6, q1
	WORD  $0xf2488dc6 // vadd.f32 q12, q12, q3
	WORD  $0xf24aadc8 // vadd.f32 q13, q13, q4
	WORD  $0xf24ccdca // vadd.f32 q14, q14, q5
	WORD  $0xf24eedcc // vadd.f32 q15, q15, q6
	WORD  $0xf2400de8 // vadd.f32 q8, q8, q12
	WORD  $0xf2422dea // vadd.f32 q9, q9, q13
	WORD  $0xf2444dec // vadd.f32 q10, q10, q14
	WORD  $0xf2466dee // vadd.f32 q11, q11, q15
	SUB.S $1, R7
	BNE   cfir_8_tap

	WORD $0xf440028d // vst1.32 {d16-d19}, [r0]!
	WORD $0xf440428d // vst1.32 {d20-d23}, [r0]!
	ADD  $64, R2
	SUB  $8, R1
	CMP  $8, R1
	BGE  cfir_8

cfir_2:
	CMP  $2, R1
	BLT  cfir_1
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVW $-8, R8
	WORD $0xf2c00050 // vmov.i32 q8, #0

cfir_2_tap:
	MOVD  (R6), F0
	ADD   $8, R6
	WORD  $0xf3bc2c40 // vdup.32 q1, d0[1]
	WORD  $0xf3022154 // veor q1, q1, q2
	WORD  $0xf4658a88 // vld1.32 {d24-d25}, [r5], r8
	WORD  $0xf3b86068 // vrev64.32 q3, q12
	WORD  $0xf3e889c0 // vmul.f32 q12, q12, d0[0]
	WORD  $0xf3066d52 // vmul.f32 q3, q3, q1
	WORD  $0xf2488dc6 // vadd.f32 q12, q12, q3
	WORD  $0xf2400de8 // vadd.f32 q8, q8, q12
	SUB.S $1, R7
	BNE   cfir_2_tap

	WORD $0xf4400a8d // vst1.32 {d16-d17}, [r0]!
	ADD  $16, R2
	SUB  $2, R1
	B    cfir_2

	// At most one output is left, accumulated in F8 and F9.
cfir_1:
	CMP  $0, R1
	BEQ  cfir_done
	ADD  R9, R2, R5
	MOVW R3, R6
	MOVW R4, R7
	MOVF $0.0, F8
	MOVF $0.0, F9

cfir_1_tap:
	MOVF  0(R6), F0 // cr
	MOVF  4(R6), F1 // ci
	MOVF  0(R5), F2 // vr
	MOVF  4(R5), F3 // vi
	ADD   $8, R6
	SUB   $8, R5
	MULF  F0, F2, F4 // cr*vr
	MULF  F1, F3, F5 // ci*vi
	SUBF  F5, F4
	MULF  F0, F3, F6 // cr*vi
	MULF  F1, F2, F7 // ci*vr
	ADDF  F7, F6
	ADDF  F4, F8
	ADDF  F6, F9
	SUB.S $1, R7
	BNE   cfir_1_tap

	MOVF F8, 0(R0)
	MOVF F9, 4(R0)

cfir_done:
	RET

// func dotF32NEON(x, c []float32) float32
//
// q8 holds accumulators 0-3 and q9 accumulators 4-7. Adding q9 to q8 and then
// the high half of q8 to the low half leaves (t0+t2, t1+t3), the reference's
// combine8 before its last add.
TEXT ·dotF32NEON(SB), NOSPLIT, $0-28
	MOVW x_base+0(FP), R0
	MOVW c_base+12(FP), R1
	MOVW c_len+16(FP), R2

	WORD $0xf2c00050 // vmov.i32 q8, #0
	WORD $0xf2c02050 // vmov.i32 q9, #0

	CMP $8, R2
	BLT dot_combine

dot_8:
	WORD $0xf460828d // vld1.32 {d24-d27}, [r0]!
	WORD $0xf461c28d // vld1.32 {d28-d31}, [r1]!
	WORD $0xf3488dfc // vmul.f32 q12, q12, q14
	WORD $0xf34aadfe // vmul.f32 q13, q13, q15
	WORD $0xf2400de8 // vadd.f32 q8, q8, q12
	WORD $0xf2422dea // vadd.f32 q9, q9, q13
	SUB  $8, R2
	CMP  $8, R2
	BGE  dot_8

dot_combine:
	WORD $0xf2400de2 // vadd.f32 q8, q8, q9
	WORD $0xf2400da1 // vadd.f32 d16, d16, d17
	WORD $0xec543b30 // vmov r3, r4, d16
	MOVW R3, F0
	MOVW R4, F1
	ADDF F1, F0

	CMP $0, R2
	BEQ dot_done

dot_1:
	MOVF  (R0), F1
	MOVF  (R1), F2
	ADD   $4, R0
	ADD   $4, R1
	MULF  F2, F1
	ADDF  F1, F0
	SUB.S $1, R2
	BNE   dot_1

dot_done:
	MOVF F0, ret+24(FP)
	RET

// func cdotRealNEON(x []complex64, c []float32) complex64
//
// A block is eight complex samples in q12-q15 against the coefficients
// duplicated to (c0, c0, c1, c1) and so on, which leaves q8 = (re0, im0, re1,
// im1), q9 = (re2, im2, re3, im3), q10 the same for samples 4 and 5 and q11
// for 6 and 7. q8+q10 and q9+q11 are combine8's first step for both parts,
// their sum its second, and the two halves of that its last.
TEXT ·cdotRealNEON(SB), NOSPLIT, $0-32
	MOVW x_base+0(FP), R0
	MOVW c_base+12(FP), R1
	MOVW c_len+16(FP), R2

	WORD $0xf2c00050 // vmov.i32 q8, #0
	WORD $0xf2c02050 // vmov.i32 q9, #0
	WORD $0xf2c04050 // vmov.i32 q10, #0
	WORD $0xf2c06050 // vmov.i32 q11, #0

	CMP $8, R2
	BLT cdot_combine

cdot_8:
	WORD $0xf460828d // vld1.32 {d24-d27}, [r0]!
	WORD $0xf460c28d // vld1.32 {d28-d31}, [r0]!
	WORD $0xf421828d // vld1.32 {d8-d11}, [r1]!
	WORD $0xf228c158 // vorr q6, q4, q4
	WORD $0xf22ae15a // vorr q7, q5, q5
	WORD $0xf3ba81cc // vzip.32 q4, q6
	WORD $0xf3baa1ce // vzip.32 q5, q7
	WORD $0xf3488dd8 // vmul.f32 q12, q12, q4
	WORD $0xf34aaddc // vmul.f32 q13, q13, q6
	WORD $0xf34ccdda // vmul.f32 q14, q14, q5
	WORD $0xf34eedde // vmul.f32 q15, q15, q7
	WORD $0xf2400de8 // vadd.f32 q8, q8, q12
	WORD $0xf2422dea // vadd.f32 q9, q9, q13
	WORD $0xf2444dec // vadd.f32 q10, q10, q14
	WORD $0xf2466dee // vadd.f32 q11, q11, q15
	SUB  $8, R2
	CMP  $8, R2
	BGE  cdot_8

cdot_combine:
	WORD $0xf2400de4 // vadd.f32 q8, q8, q10
	WORD $0xf2422de6 // vadd.f32 q9, q9, q11
	WORD $0xf2400de2 // vadd.f32 q8, q8, q9
	WORD $0xf2400da1 // vadd.f32 d16, d16, d17
	WORD $0xec543b30 // vmov r3, r4, d16
	MOVW R3, F0
	MOVW R4, F1

	CMP $0, R2
	BEQ cdot_done

cdot_1:
	MOVF  0(R0), F2
	MOVF  4(R0), F3
	MOVF  (R1), F4
	ADD   $8, R0
	ADD   $4, R1
	MULF  F4, F2
	MULF  F4, F3
	ADDF  F2, F0
	ADDF  F3, F1
	SUB.S $1, R2
	BNE   cdot_1

cdot_done:
	MOVF F0, ret_real+24(FP)
	MOVF F1, ret_imag+28(FP)
	RET
