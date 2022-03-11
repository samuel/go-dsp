#include "textflag.h"

TEXT ·Ui8toi16(SB), NOSPLIT, $0
    B ·ui8toi16(SB)

TEXT ·Ui8toi16b(SB), NOSPLIT, $0
	B ·ui8toi16b(SB)

TEXT ·Ui8tof32(SB), NOSPLIT, $0
	MOVD	input(FP), R0
	MOVD	input_len+8(FP), R1
	MOVD	output+24(FP), R2
	MOVD	output_len+32(FP), R3

    CMP     R3, R1
    BLT     ui8tof32_min_len
    MOVD    R3, R1
ui8tof32_min_len:

#define BLOCK_SIZE 32

	MOVW	$0x80, R3
	WORD	$0x4e010c60 // dup v0.16b, w3

    CMP     $BLOCK_SIZE, R1
    BLT     ui8tof32_scalar

ui8tof32_simd_loop:
    WORD    $0xad400801 // ldp q1, q2, [x0]
    ADD     $BLOCK_SIZE, R0
	WORD	$0x6e208421 // sub v1.16b, v1.16b, v0.16b
	WORD	$0x6e208442 // sub v2.16b, v2.16b, v0.16b
	WORD    $0xf9804001 // prfm pldl1strm, [x0, 128]
	WORD	$0x0f08a42a // sxtl v10.8h, v1.8b
	WORD	$0x4f08a42b // sxtl2 v11.8h, v1.16b
	WORD	$0x0f08a44c // sxtl v12.8h, v2.8b
	WORD	$0x4f08a44d // sxtl2 v13.8h, v2.16b
	WORD	$0x0f10a554 // sxtl v20.4s, v10.4h
	WORD	$0x4f10a555 // sxtl2 v21.4s, v10.8h
	WORD	$0x0f10a576 // sxtl v22.4s, v11.4h
	WORD	$0x4f10a577 // sxtl2 v23.4s, v11.8h
	WORD	$0x0f10a598 // sxtl v24.4s, v12.4h
	WORD	$0x4f10a599 // sxtl2 v25.4s, v12.8h
	WORD	$0x0f10a5ba // sxtl v26.4s, v13.4h
	WORD	$0x4f10a5bb // sxtl2 v27.4s, v13.8h
	//WORD	$0x4e21da81 // scvtf v1.4s, v20.4s
	//WORD	$0x4e21daa2 // scvtf v2.4s, v21.4s
	WORD	$0x4e21da89 // scvtf v9.4s, v20.4s
	WORD	$0x4e21daaa // scvtf v10.4s, v21.4s
	WORD	$0x4e21dac3 // scvtf v3.4s, v22.4s
	WORD	$0x4e21dae4 // scvtf v4.4s, v23.4s
    //WORD    $0xad000841 // stp q1, q2, [x2]
	WORD    $0xad002849 // stp q9, q10, [x2]
	WORD	$0x4e21db05 // scvtf v5.4s, v24.4s
	WORD	$0x4e21db26 // scvtf v6.4s, v25.4s
    WORD    $0xad011043 // stp q3, q4, [x2,32]
	WORD	$0x4e21db47 // scvtf v7.4s, v26.4s
	WORD	$0x4e21db68 // scvtf v8.4s, v27.4s
	WORD    $0xad021845 // stp q5, q6, [x2,64]
	//WORD    $0xad032047 // stp q7, q8, [x2,96]
    ADD     $(BLOCK_SIZE*4), R2
    SUB     $BLOCK_SIZE, R1
	WORD    $0xad3f2047 // stp q7, q8, [x2,-32]
    CMP     $BLOCK_SIZE, R1
    BGE     ui8tof32_simd_loop

ui8tof32_scalar:
    CMP     ZR, R1
    BEQ     ui8tof32_done

ui8tof32_scalar_loop:
	MOVBU	(R0), R5
	SUB		$128, R5, R5
	SCVTFS	R5, F0
	FMOVS	F0, (R2)
	ADD		$1, R0
	ADD		$4, R2
    SUBS    $1, R1
	BNE     ui8tof32_scalar_loop
ui8tof32_done:
	RET

TEXT ·I8tof32(SB), NOSPLIT, $0
	B ·i8tof32(SB)

TEXT ·Ui8toc64(SB), NOSPLIT, $0
	B ·ui8toc64(SB)

TEXT ·F32toi16(SB), NOSPLIT, $0
    B ·f32toi16(SB)

TEXT ·F32toi16ble(SB), NOSPLIT, $0
	B ·f32toi16ble(SB)

TEXT ·I16bleToF64(SB), NOSPLIT, $0
	B ·i16bleToF64(SB)

TEXT ·I16bleToF32(SB), NOSPLIT, $0
	B ·i16bleToF32(SB)
