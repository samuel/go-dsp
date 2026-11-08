//go:build !goexperiment.simd

#include "textflag.h"

// func U8ToF32(dst []float32, src []byte)
//
// GOEXPERIMENT=simd is off by default, so without this file a plain `go build`
// on arm64 would fall through to the scalar loop. NEON is part of the arm64
// baseline and needs no runtime dispatch. The archsimd version in
// conversion_simd_arm64.go carries the same steps: subtract the 128 bias as
// bytes, sign-extend in two steps, then convert.
//
// It is not merely a fallback. The PRFM below has no archsimd equivalent, and
// it is what makes this version 17% faster than the Go one at 64K samples on an
// M2 Pro, where the loop is memory-bound. The Go version is 6-17% faster from
// 256 samples to 16K, where it is not.
TEXT ·U8ToF32(SB), NOSPLIT, $0
	MOVD	src_base+24(FP), R0
	MOVD	src_len+32(FP), R1
	MOVD	dst+0(FP), R2
	MOVD	dst_len+8(FP), R3

    CMP     R3, R1
	CSEL	LT, R1, R3, R1

#define UI8_TO_F32_BLOCK_SIZE 32

	MOVW	$0x80, R3
	VDUP	R3, V0.B16

    CMP     $UI8_TO_F32_BLOCK_SIZE, R1
    BLT     u8tof32_scalar

u8tof32_simd_loop:
    FLDPQ	(R0), (F1, F2)
    ADD     $UI8_TO_F32_BLOCK_SIZE, R0
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
	//VSCVTF	V20.S4, V1.S4
	//VSCVTF	V21.S4, V2.S4
	VSCVTF	V20.S4, V9.S4
	VSCVTF	V21.S4, V10.S4
	VSCVTF	V22.S4, V3.S4
	VSCVTF	V23.S4, V4.S4
    //FSTPQ	(F1, F2), (R2)
	FSTPQ	(F9, F10), (R2)
	VSCVTF	V24.S4, V5.S4
	VSCVTF	V25.S4, V6.S4
    FSTPQ	(F3, F4), 32(R2)
	VSCVTF	V26.S4, V7.S4
	VSCVTF	V27.S4, V8.S4
	FSTPQ	(F5, F6), 64(R2)
	//FSTPQ	(F7, F8), 96(R2)
    ADD     $(UI8_TO_F32_BLOCK_SIZE*4), R2
    SUB     $UI8_TO_F32_BLOCK_SIZE, R1
	FSTPQ	(F7, F8), -32(R2)
    CMP     $UI8_TO_F32_BLOCK_SIZE, R1
    BGE     u8tof32_simd_loop

u8tof32_scalar:
    CMP     ZR, R1
    BEQ     u8tof32_done

u8tof32_scalar_loop:
	MOVBU	(R0), R5
	SUB		$128, R5, R5
	SCVTFS	R5, F0
	FMOVS	F0, (R2)
	ADD		$1, R0
	ADD		$4, R2
    SUBS    $1, R1
	BNE     u8tof32_scalar_loop
u8tof32_done:
	RET
