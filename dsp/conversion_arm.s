#include "textflag.h"

TEXT ·Ui8toi16(SB), NOSPLIT, $0
	MOVW input+0(FP), R1
	MOVW input_len+4(FP), R2
	MOVW output+12(FP), R3
	MOVW output_len+16(FP), R4

	// Choose the shortest length
	CMP     R2, R4
	MOVW.LT R4, R2

	// If no input then skip loop
	TEQ $0, R2
	BEQ ui8toi16_done
	ADD R1, R2

ui8toi16_loop:
	MOVBU 0(R1), R0
	ADD   $1, R1
	SUB   $128, R0
	MOVBU R0, 0(R3)
	MOVBU R0, 1(R3)
	ADD   $2, R3
	CMP   R2, R1
	BLT   ui8toi16_loop

ui8toi16_done:
	RET

TEXT ·Ui8toi16b(SB), NOSPLIT, $0
	MOVW output_len+16(FP), R4
	MOVW R4>>1, R4
	MOVW R4, output_len+16(FP)
	B    ·Ui8toi16(SB)

TEXT ·Ui8tof32(SB), NOSPLIT, $0
	MOVW input+0(FP), R1
	MOVW input_len+4(FP), R2
	MOVW output+12(FP), R3
	MOVW output_len+16(FP), R0

	// Choose the shortest length
	CMP     R2, R0
	MOVW.LT R0, R2

	// If no input then skip loop
	CMP $0, R2
	BEQ ui8tof32_done

	MOVBU ·HaveNEON+0(SB), R0
	CMP   $0, R0
	BNE   ui8tof32_neon

	AND $(~3), R2, R0
	ADD R1, R2
	TEQ $0, R0
	BEQ ui8tof32_tail
	ADD R1, R0

	MOVW $0x80808080, R8

ui8tof32_loop:
	// This is slower on Raspberry Pi but faster on Udoo Quad (which uses NEON anyway)
	// MOVBU	0(R1), R4
	// MOVBU	1(R1), R5
	// MOVBU	2(R1), R6
	// MOVBU	3(R1), R7
	// ADD	$4, R1
	// SUB	$128, R4
	// SUB	$128, R5
	// SUB	$128, R6
	// SUB	$128, R7

	// This is faster on Raspberry Pi but slower on Udoo Quad (which uses NEON anyway)
	MOVW (R1), R4
	ADD  $4, R1
	WORD $0xe6544ff8 // usub8 r4, r4, r8
	WORD $0xe6af5474 // sxtb r5, r4, ror #8
	WORD $0xe6af6874 // sxtb r6, r4, ror #16
	WORD $0xe6af7c74 // sxtb r7, r4, ror #24
	WORD $0xe6af4074 // sxtb r4, r4

	WORD $0xec454a1e // vmov s28, s29, r4, r5
	WORD $0xec476a1f // vmov s30, s31, r6, r7
	WORD $0xeeb80ace // vcvt.f32.s32 s0, s28
	WORD $0xeef80aee // vcvt.f32.s32 s1, s29
	WORD $0xeeb81acf // vcvt.f32.s32 s2, s30
	WORD $0xeef81aef // vcvt.f32.s32 s3, s31

	WORD $0xeca30a04   // vstmia r3!, {s0, s1, s2, s3}
	CMP  R0, R1
	BLT  ui8tof32_loop

	B ui8tof32_tail

	////////////// Neon ////////////

ui8tof32_neon:
	MOVW $128, R0
	WORD $0xeee00b10 // vdup.8 q0, r0

	AND $(~(16*4-1)), R2, R4
	ADD R1, R2
	TEQ $0, R4
	BEQ ui8tof32_tail
	ADD R1, R4

ui8tof32_neon_loop:
	WORD $0xf461c28d // vld1.32 {d28, d29, d30, d31}, [r1]!

	// WORD	$0xf461c2bd // vld1.32 {d28, d29, d30, d31}, [r1:256]!
	WORD $0xf3cc4280 // vsubl.u8 q10, d28, d0
	WORD $0xf3cd6280 // vsubl.u8 q11, d29, d0
	WORD $0xf3ce8280 // vsubl.u8 q12, d30, d0
	WORD $0xf3cfa280 // vsubl.u8 q13, d31, d0
	WORD $0xf2902a34 // vmovl.s16 q1, d20
	WORD $0xf2904a35 // vmovl.s16 q2, d21
	WORD $0xf2906a36 // vmovl.s16 q3, d22
	WORD $0xf2908a37 // vmovl.s16 q4, d23
	WORD $0xf290aa38 // vmovl.s16 q5, d24
	WORD $0xf290ca39 // vmovl.s16 q6, d25
	WORD $0xf290ea3a // vmovl.s16 q7, d26
	WORD $0xf2d00a3b // vmovl.s16 q8, d27
	WORD $0xf3bb2642 // vcvt.f32.s32 q1, q1
	WORD $0xf3bb4644 // vcvt.f32.s32 q2, q2
	WORD $0xf3bb6646 // vcvt.f32.s32 q3, q3
	WORD $0xf3bb8648 // vcvt.f32.s32 q4, q4
	WORD $0xf403228d // vst1.32 {d2, d3, d4, d5}, [r3]!
	WORD $0xf3bba64a // vcvt.f32.s32 q5, q5
	WORD $0xf3bbc64c // vcvt.f32.s32 q6, q6
	WORD $0xf403628d // vst1.32 {d6, d7, d8, d9}, [r3]!
	WORD $0xf3bbe64e // vcvt.f32.s32 q7, q7
	WORD $0xf3fb0660 // vcvt.f32.s32 q8, q8
	WORD $0xf403a28d // vst1.32 {d10, d11, d12, d13}, [r3]!
	WORD $0xf403e28d // vst1.32 {d14, d15, d16, d17}, [r3]!

	CMP R4, R1
	BLT ui8tof32_neon_loop

ui8tof32_tail:
	CMP R1, R2
	BEQ ui8tof32_done

ui8tof32_tail_loop:
	MOVBU 0(R1), R4
	SUB   $128, R4
	MOVWF R4, F0
	ADD   $1, R1
	WORD  $0xeca30a01        // vstmia     r3!, {s0}
	CMP   R2, R1
	BLT   ui8tof32_tail_loop

ui8tof32_done:
	RET

// TODO
TEXT ·I8tof32(SB), NOSPLIT, $0
	B ·i8tof32(SB)

TEXT ·Ui8toc64(SB), NOSPLIT, $0
	MOVW input_len+4(FP), R2
	AND  $(~1), R2
	MOVW R2, input_len+4(FP)
	MOVW output_len+16(FP), R0
	MOVW R0<<1, R0
	MOVW R0, output_len+16(FP)
	B    ·Ui8tof32(SB)

TEXT ·F32toi16(SB), NOSPLIT, $0
	MOVW input+0(FP), R1
	MOVW input_len+4(FP), R2
	MOVW output+12(FP), R3
	MOVW output_len+16(FP), R0
	MOVF scale+24(FP), F0

	// Choose the shortest length
	CMP     R2, R0
	MOVW.LT R0, R2

	// If no input then we are done
	TEQ $0, R2
	BEQ f32toi16_done

	MOVW R2, R7
	ADD  R2<<2, R1, R2

	// R1 = input
	// R2 = end of output
	// R3 = output
	// R7 = count

	MOVBU ·UseVector+0(SB), R0
	TEQ   $0, R0
	BNE   f32toi16_vector

	//////////////// VFP Scalar /////////////

	AND $(~3), R7
	TEQ $0, R7
	BEQ f32toi16_tail
	ADD R7<<2, R1, R7 // R7 = end of output truncated to block size

f32toi16_scalar_loop:
	WORD $0xecb11a04 // vldmia r1!, {s2, s3, s4, s5}
	WORD $0xee211a00 // vmul.f32 s2, s2, s0
	WORD $0xee611a80 // vmul.f32 s3, s3, s0
	WORD $0xee222a00 // vmul.f32 s4, s4, s0
	WORD $0xee622a80 // vmul.f32 s5, s5, s0
	WORD $0xeebd1ac1 // vcvt.s32.f32 s2, s2
	WORD $0xeefd1ae1 // vcvt.s32.f32 s3, s3
	WORD $0xeebd2ac2 // vcvt.s32.f32 s4, s4
	WORD $0xeefd2ae2 // vcvt.s32.f32 s5, s5
	WORD $0xec540a11 // vmov r0, r4, s2, s3
	MOVH R0, 0(R3)
	MOVH R4, 2(R3)
	WORD $0xec5b8a12 // vmov r8, r11, s4, s5
	MOVH R8, 4(R3)
	MOVH R11, 6(R3)
	ADD  $8, R3

	CMP R7, R1
	BLT f32toi16_scalar_loop

	B f32toi16_tail

	///////////// VFP Vector //////////////

f32toi16_vector:
	AND $(~7), R7
	TEQ $0, R7
	BEQ f32toi16_tail
	ADD R7<<2, R1, R7 // R7 = end of output truncated to block size

	PLD (R1)
	PLD 64(R1)
	PLD (2*64)(R1)
	PLD (3*64)(R1)

	// Set vector length to 8
	WORD $0xeef10a10            // vmrs r0, fpscr
	BIC  $((7<<16)|(3<<20)), R0
	ORR  $((7<<16)|(0<<20)), R0
	WORD $0xeee10a10            // fmxr fpscr, r0

f32toi16_vector_loop:
	PLD  (4*64)(R1)
	WORD $0xecb14a08 // vldmia r1!, {s8-s15}
	WORD $0xee244a00 // vmul.f32 s8, s8, s0
	WORD $0xeebd4ac4 // vcvt.s32.f32 s8, s8
	WORD $0xeefd4ae4 // vcvt.s32.f32 s9, s9
	WORD $0xeebd5ac5 // vcvt.s32.f32 s10, s10
	WORD $0xeefd5ae5 // vcvt.s32.f32 s11, s11
	WORD $0xec540a14 // vmov r0, r4, s8, s9
	WORD $0xec5b8a15 // vmov r8, r11, s10, s11
	MOVH R0, 0(R3)
	MOVH R4, 2(R3)
	MOVH R8, 4(R3)
	MOVH R11, 6(R3)
	WORD $0xeebd6ac6 // vcvt.s32.f32 s12, s12
	WORD $0xeefd6ae6 // vcvt.s32.f32 s13, s13
	WORD $0xeebd7ac7 // vcvt.s32.f32 s14, s14
	WORD $0xeefd7ae7 // vcvt.s32.f32 s15, s15
	WORD $0xec540a16 // vmov r0, r4, s12, s13
	WORD $0xec5b8a17 // vmov r8, r11, s14, s15
	MOVH R0, 8(R3)
	MOVH R4, 10(R3)
	MOVH R8, 12(R3)
	MOVH R11, 14(R3)
	ADD  $16, R3

	CMP R7, R1
	BLT f32toi16_vector_loop

	// Clear vector mode
	WORD $0xeef10a10            // vmrs r0, fpscr
	BIC  $((7<<16)|(3<<20)), R0
	WORD $0xeee10a10            // fmxr fpscr, r0

f32toi16_tail:
	CMP R1, R2
	BEQ f32toi16_done

f32toi16_tail_loop:
	MOVF  0(R1), F1
	ADD   $4, R1
	MULF  F0, F1
	MOVFW F1, R0
	MOVHU R0, (R3)
	ADD   $2, R3
	CMP   R2, R1
	BLT   f32toi16_tail_loop

f32toi16_done:
	RET

// TODO: detect endianess and use non-native order writes on big-endian
TEXT ·F32toi16ble(SB), NOSPLIT, $0
	MOVW output_len+16(FP), R0
	MOVW R0>>1, R0
	MOVW R0, output_len+16(FP)
	B    ·F32toi16(SB)

TEXT ·I16bleToF64(SB), NOSPLIT, $0
	B ·i16bleToF64(SB)

TEXT ·I16bleToF32(SB), NOSPLIT, $0
	B ·i16bleToF32(SB)
