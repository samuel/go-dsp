//go:build !goexperiment.simd

#include "textflag.h"

// NEON is part of the arm64 baseline, so these need no runtime dispatch. The
// archsimd versions in decode_simd_arm64.go and encode_simd_arm64.go run the
// same steps; these keep a PRFM, which archsimd cannot express and which pays
// off once the loop is memory-bound.
//
// Decoding divides by 2^(bits-1). The vector loops do it in the convert itself,
// with SCVTF's fixed-point form taking bits-1 fraction bits; the scalar tails
// multiply by the power of two after converting. The integers are exact in a
// float32 and the divisor is a power of two, so both match the reference bit
// for bit. The Go assembler has no mnemonic for the fixed-point SCVTF, so it
// is encoded with WORD: 0x4f000000 | (64-fbits)<<16 | 0xe400 | Vn<<5 | Vd.

#define SCALE_8  $0x3c000000 // 2^-7 as float32
#define SCALE_16 $0x38000000 // 2^-15 as float32
#define FULL_16  $0x47000000 // 2^15 as float32

// func u8ToF32(dst []float32, src []byte)
TEXT ·u8ToF32(SB), NOSPLIT, $0-48
	MOVD	src_base+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst_base+0(FP), R2
	MOVD	dst_len+8(FP), R3
	CMP	R3, R1
	CSEL	LT, R1, R3, R1

	MOVW	$0x80, R3
	VDUP	R3, V0.B16
	MOVW	SCALE_8, R3
	VDUP	R3, V31.S4

	CMP	$32, R1
	BLT	u8_scalar

u8_loop:
	FLDPQ	(R0), (F1, F2)
	ADD	$32, R0
	// The bias comes off as a wrapping byte subtract: s-128 as int8.
	VSUB	V0.B16, V1.B16, V1.B16
	VSUB	V0.B16, V2.B16, V2.B16
	PRFM	128(R0), PLDL1STRM
	VSXTL	V1.B8, V10.H8
	VSXTL2	V1.B16, V11.H8
	VSXTL	V2.B8, V12.H8
	VSXTL2	V2.B16, V13.H8
	VSXTL	V10.H4, V20.S4
	VSXTL2	V10.H8, V21.S4
	VSXTL	V11.H4, V22.S4
	VSXTL2	V11.H8, V23.S4
	VSXTL	V12.H4, V24.S4
	VSXTL2	V12.H8, V25.S4
	VSXTL	V13.H4, V26.S4
	VSXTL2	V13.H8, V27.S4
	WORD	$0x4f39e681 // scvtf v1.4s, v20.4s, #7
	WORD	$0x4f39e6a2 // scvtf v2.4s, v21.4s, #7
	WORD	$0x4f39e6c3 // scvtf v3.4s, v22.4s, #7
	WORD	$0x4f39e6e4 // scvtf v4.4s, v23.4s, #7
	FSTPQ	(F1, F2), (R2)
	WORD	$0x4f39e705 // scvtf v5.4s, v24.4s, #7
	WORD	$0x4f39e726 // scvtf v6.4s, v25.4s, #7
	FSTPQ	(F3, F4), 32(R2)
	WORD	$0x4f39e747 // scvtf v7.4s, v26.4s, #7
	WORD	$0x4f39e768 // scvtf v8.4s, v27.4s, #7
	FSTPQ	(F5, F6), 64(R2)
	FSTPQ	(F7, F8), 96(R2)
	ADD	$128, R2
	SUB	$32, R1
	CMP	$32, R1
	BGE	u8_loop

u8_scalar:
	CBZ	R1, u8_done

u8_scalar_loop:
	MOVBU	(R0), R5
	SUB	$128, R5, R5
	SCVTFS	R5, F0
	FMULS	F31, F0, F0
	FMOVS	F0, (R2)
	ADD	$1, R0
	ADD	$4, R2
	SUBS	$1, R1
	BNE	u8_scalar_loop

u8_done:
	RET

// func i8ToF32(dst []float32, src []byte)
TEXT ·i8ToF32(SB), NOSPLIT, $0-48
	MOVD	src_base+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst_base+0(FP), R2
	MOVD	dst_len+8(FP), R3
	CMP	R3, R1
	CSEL	LT, R1, R3, R1

	MOVW	SCALE_8, R3
	VDUP	R3, V31.S4

	CMP	$32, R1
	BLT	i8_scalar

i8_loop:
	FLDPQ	(R0), (F1, F2)
	ADD	$32, R0
	PRFM	128(R0), PLDL1STRM
	VSXTL	V1.B8, V10.H8
	VSXTL2	V1.B16, V11.H8
	VSXTL	V2.B8, V12.H8
	VSXTL2	V2.B16, V13.H8
	VSXTL	V10.H4, V20.S4
	VSXTL2	V10.H8, V21.S4
	VSXTL	V11.H4, V22.S4
	VSXTL2	V11.H8, V23.S4
	VSXTL	V12.H4, V24.S4
	VSXTL2	V12.H8, V25.S4
	VSXTL	V13.H4, V26.S4
	VSXTL2	V13.H8, V27.S4
	WORD	$0x4f39e681 // scvtf v1.4s, v20.4s, #7
	WORD	$0x4f39e6a2 // scvtf v2.4s, v21.4s, #7
	WORD	$0x4f39e6c3 // scvtf v3.4s, v22.4s, #7
	WORD	$0x4f39e6e4 // scvtf v4.4s, v23.4s, #7
	FSTPQ	(F1, F2), (R2)
	WORD	$0x4f39e705 // scvtf v5.4s, v24.4s, #7
	WORD	$0x4f39e726 // scvtf v6.4s, v25.4s, #7
	FSTPQ	(F3, F4), 32(R2)
	WORD	$0x4f39e747 // scvtf v7.4s, v26.4s, #7
	WORD	$0x4f39e768 // scvtf v8.4s, v27.4s, #7
	FSTPQ	(F5, F6), 64(R2)
	FSTPQ	(F7, F8), 96(R2)
	ADD	$128, R2
	SUB	$32, R1
	CMP	$32, R1
	BGE	i8_loop

i8_scalar:
	CBZ	R1, i8_done

i8_scalar_loop:
	MOVB	(R0), R5
	SCVTFS	R5, F0
	FMULS	F31, F0, F0
	FMOVS	F0, (R2)
	ADD	$1, R0
	ADD	$4, R2
	SUBS	$1, R1
	BNE	i8_scalar_loop

i8_done:
	RET

// func i16LEToF32(dst []float32, src []byte)
TEXT ·i16LEToF32(SB), NOSPLIT, $0-48
	MOVD	src_base+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst_base+0(FP), R2
	MOVD	dst_len+8(FP), R3
	LSR	$1, R1, R1
	CMP	R3, R1
	CSEL	LT, R1, R3, R1

	MOVW	SCALE_16, R3
	VDUP	R3, V31.S4

	CMP	$16, R1
	BLT	i16_scalar

i16_loop:
	FLDPQ	(R0), (F1, F2)
	ADD	$32, R0
	PRFM	128(R0), PLDL1STRM
	VSXTL	V1.H4, V20.S4
	VSXTL2	V1.H8, V21.S4
	VSXTL	V2.H4, V22.S4
	VSXTL2	V2.H8, V23.S4
	WORD	$0x4f31e683 // scvtf v3.4s, v20.4s, #15
	WORD	$0x4f31e6a4 // scvtf v4.4s, v21.4s, #15
	WORD	$0x4f31e6c5 // scvtf v5.4s, v22.4s, #15
	WORD	$0x4f31e6e6 // scvtf v6.4s, v23.4s, #15
	FSTPQ	(F3, F4), (R2)
	FSTPQ	(F5, F6), 32(R2)
	ADD	$64, R2
	SUB	$16, R1
	CMP	$16, R1
	BGE	i16_loop

i16_scalar:
	CBZ	R1, i16_done

i16_scalar_loop:
	MOVH	(R0), R5
	SCVTFS	R5, F0
	FMULS	F31, F0, F0
	FMOVS	F0, (R2)
	ADD	$2, R0
	ADD	$4, R2
	SUBS	$1, R1
	BNE	i16_scalar_loop

i16_done:
	RET

// func f32ToI16LE(dst []byte, src []float32)
//
// Each sample is multiplied by 2^15, which is exact or overflows to an
// infinity, then FCVTNS converts it, rounding to nearest even, saturating at
// the int32 range and turning NaN into zero. SQXTN saturates to int16.
// Saturating after rounding clips at the same codes as the reference, which
// clips first: the rails are integers.
//
// The Go assembler has no FCVTNS, so it is encoded with WORD.
TEXT ·f32ToI16LE(SB), NOSPLIT, $0-48
	MOVD	src_base+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst_base+0(FP), R2
	MOVD	dst_len+8(FP), R3
	LSR	$1, R3, R3
	CMP	R3, R1
	CSEL	LT, R1, R3, R1

	MOVW	FULL_16, R3
	VDUP	R3, V31.S4

	CMP	$16, R1
	BLT	f32i16_scalar

f32i16_loop:
	FLDPQ	(R0), (F1, F2)
	FLDPQ	32(R0), (F3, F4)
	ADD	$64, R0
	PRFM	256(R0), PLDL1STRM
	VFMUL	V31.S4, V1.S4, V1.S4
	VFMUL	V31.S4, V2.S4, V2.S4
	VFMUL	V31.S4, V3.S4, V3.S4
	VFMUL	V31.S4, V4.S4, V4.S4
	WORD	$0x4e21a821 // FCVTNS V1.4S, V1.4S
	WORD	$0x4e21a842 // FCVTNS V2.4S, V2.4S
	WORD	$0x4e21a863 // FCVTNS V3.4S, V3.4S
	WORD	$0x4e21a884 // FCVTNS V4.4S, V4.4S
	VSQXTN	V1.S4, V5.H4
	VSQXTN2	V2.S4, V5.H8
	VSQXTN	V3.S4, V6.H4
	VSQXTN2	V4.S4, V6.H8
	FSTPQ	(F5, F6), (R2)
	ADD	$32, R2
	SUB	$16, R1
	CMP	$16, R1
	BGE	f32i16_loop

f32i16_scalar:
	CBZ	R1, f32i16_done
	MOVW	$32767, R6
	MOVW	$-32768, R7

f32i16_scalar_loop:
	FMOVS	(R0), F0
	FMULS	F31, F0, F0
	WORD	$0x1e200005 // FCVTNS W5, S0
	CMPW	R6, R5
	CSELW	GT, R6, R5, R5
	CMPW	R7, R5
	CSELW	LT, R7, R5, R5
	MOVH	R5, (R2)
	ADD	$4, R0
	ADD	$2, R2
	SUBS	$1, R1
	BNE	f32i16_scalar_loop

f32i16_done:
	RET
