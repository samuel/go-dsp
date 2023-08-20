#include "textflag.h"

#define Inf32 0x7f800000
#define NegInf32 0xff800000

TEXT ·FastAtan2(SB), NOSPLIT, $0
    B ·fastAtan2(SB)

TEXT ·FastAtan2_2(SB), NOSPLIT, $0
    B ·fastAtan2_2(SB)

TEXT ·VScaleF32(SB), NOSPLIT, $0
	MOVD	input(FP), R0
	MOVD	input_len+8(FP), R1
	MOVD	output+24(FP), R2
	MOVD	output_len+32(FP), R3
    FMOVS   scale+48(FP), F0

    CMP     R3, R1
    BLT     vscalef32_min_len
    MOVD    R3, R1
vscalef32_min_len:

#define BLOCK_SIZE 16

    CMP     $BLOCK_SIZE, R1
    BLT     vscalef32_scaler
vscalef32_simd_loop:
    //VLD1    (R0), [V1.S4]
    //WORD    $0x3dc00001 // ldr q1, [x0]
    //ADD     $16, R0

    //WORD    $0xf9802003 // prfm pldl2strm, [x0, 64]
    //WORD    $0xf980a000 // prfm pldl1keep, [x0, 320]
    //WORD    $0xf980a001 // prfm pldl1strm, [x0, 320]
    WORD    $0xf9804001 // prfm pldl1strm, [x0, 128]
    //WORD    $0xf9804000 // prfm pldl1keep, [x0, 128]

    //WORD    $0x4c402801 // ld1 {v1.4s,v2.4s,v3.4s,v4.4s}, [x0]
    WORD    $0xad400801 // ldp q1, q2, [x0]
    WORD    $0xad411003 // ldp q3, q4, [x0,32]
    //WORD    $0xad421805 // ldp q5, q6, [x0,64]
    ADD     $(BLOCK_SIZE*4), R0

    WORD    $0x4f809021    // fmul v1.4s, v1.4s, v0.s[0]
    WORD    $0x4f809042    // fmul v2.4s, v2.4s, v0.s[0]
    WORD    $0x4f809063    // fmul v3.4s, v3.4s, v0.s[0]
    WORD    $0x4f809084    // fmul v4.4s, v4.4s, v0.s[0]
    //WORD    $0x4f8090a5    // fmul v5.4s, v5.4s, v0.s[0]
    //WORD    $0x4f8090c6    // fmul v6.4s, v6.4s, v0.s[0]

    //VST1    [V1.S4], (R2)
    //WORD    $0x3d800041 // str q1, [x2]
    //ADD     $16, R2

    //WORD    $0x4c00a841 // st1 {v1.4s,v2.4s}, [x2]
    WORD    $0xad000841 // stp q1, q2, [x2]
    WORD    $0xad011043 // stp q3, q4, [x2,32]
    //WORD    $0xad021845 // stp q5, q6, [x2,64]
    ADD     $(BLOCK_SIZE*4), R2

    SUB     $BLOCK_SIZE, R1
    CMP     $BLOCK_SIZE, R1
    BGE     vscalef32_simd_loop

vscalef32_scaler:
    CMP     ZR, R1
    BEQ     vscalef32_done
vscalef32_scaler_loop:
	FMOVS.P	4(R0), F1
    FMULS   F0, F1, F1
    FMOVS.P F1, 4(R2)
    SUBS    $1, R1
	BNE     vscalef32_scaler_loop
vscalef32_done:
	RET

TEXT ·VMulC64xF32(SB), NOSPLIT, $0
	B ·vMulC64xF32(SB)

TEXT ·VAbsC64(SB), NOSPLIT, $0
    B ·vAbsC64(SB)

TEXT ·VMaxF32(SB), NOSPLIT, $0
	MOVD	input(FP), R0
	MOVD	input_len+8(FP), R1
	MOVW    $NegInf32, R2
	FMOVS   R2, F31

#undef BLOCK_SIZE
#define BLOCK_SIZE 16

    CMP     $(8+BLOCK_SIZE), R1
    BLT     vmaxf32_scaler

    //VLD1.P  16(R0), [V0.S4] // ld1 {v0.4s}, [x0], #16
    WORD    $0xad401c00 // ldp q0, q7, [x0]
    ADD     $(BLOCK_SIZE/2*4), R0
    SUB     $(BLOCK_SIZE/2), R1
vmaxf32_simd_loop:
    //VLD1.P    (R0), [V1.S4,V2.S4,V3.S4,V4.S4]
    // ldp is faster than vld1
    WORD    $0xad400801     // ldp q1, q2, [x0]
    WORD    $0xad411003     // ldp q3, q4, [x0,32]
    ADD     $(BLOCK_SIZE*4), R0
    WORD    $0x4e21f400     // fmax v0.4s, v0.4s, v1.4s
    WORD    $0x4e23f4e7     // fmax v7.4s, v7.4s, v3.4s
    WORD    $0x4e22f400     // fmax v0.4s, v0.4s, v2.4s
    WORD    $0x4e24f4e7     // fmax v7.4s, v7.4s, v4.4s
    SUB     $BLOCK_SIZE, R1
    CMP     $BLOCK_SIZE, R1
    BGE     vmaxf32_simd_loop
    WORD    $0x6e30f81e     // fmaxv s30, v0.4s
    WORD    $0x6e30f8ff     // fmaxv s31, v7.4s
    FMAXS   F31, F30, F31

vmaxf32_scaler:
    CMP     ZR, R1
    BEQ     vmaxf32_done
vmaxf32_loop:
	//FMOVS.P	(R0), F1
    FMOVS   (R0), F1
    ADD     $4, R0
    FMAXS   F31, F1, F31
    SUBS    $1, R1
	BNE     vmaxf32_loop
vmaxf32_done:
	FMOVS	F31, ret+24(FP)
	RET

TEXT ·VMinF32(SB), NOSPLIT, $0
	MOVD	input(FP), R0
	MOVD	input_len+8(FP), R1
	MOVW    $Inf32, R2
	FMOVS   R2, F31

#undef BLOCK_SIZE
#define BLOCK_SIZE 16

    CMP     $(8+BLOCK_SIZE), R1
    BLT     vmaxf32_scaler

    //VLD1.P  16(R0), [V0.S4] // ld1 {v0.4s}, [x0], #16
    WORD    $0xad401c00 // ldp q0, q7, [x0]
    ADD     $(BLOCK_SIZE/2*4), R0
    SUB     $(BLOCK_SIZE/2), R1
vmaxf32_simd_loop:
    //VLD1.P    (R0), [V1.S4,V2.S4,V3.S4,V4.S4]
    // ldp is faster than vld1
    WORD    $0xad400801     // ldp q1, q2, [x0]
    WORD    $0xad411003     // ldp q3, q4, [x0,32]
    ADD     $(BLOCK_SIZE*4), R0
    WORD    $0x4ea1f400     // fmin v0.4s, v0.4s, v1.4s
    WORD    $0x4ea3f4e7     // fmin v7.4s, v7.4s, v3.4s
    WORD    $0x4ea2f400     // fmin v0.4s, v0.4s, v2.4s
    WORD    $0x4ea4f4e7     // fmin v7.4s, v7.4s, v4.4s
    SUB     $BLOCK_SIZE, R1
    CMP     $BLOCK_SIZE, R1
    BGE     vmaxf32_simd_loop
    WORD    $0x6eb0f81e     // fminv s30, v0.4s
    WORD    $0x6eb0f8ff     // fminv s31, v7.4s
    FMINS   F31, F30, F31

vmaxf32_scaler:
    CMP     ZR, R1
    BEQ     vmaxf32_done
vmaxf32_loop:
	//FMOVS.P	(R0), F1
    FMOVS   (R0), F1
    ADD     $4, R0
    FMINS   F31, F1, F31
    SUBS    $1, R1
	BNE     vmaxf32_loop
vmaxf32_done:
	FMOVS	F31, ret+24(FP)
	RET
