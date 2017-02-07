#include "textflag.h"

#define pi $3.14159265358979323846264338327950288419716939937510582097494459
#define halfPi $1.570796326794896557998981734272092580795288085938
#define negativeHalfPi $-1.570796326794896557998981734272092580795288085938

#define vmrs_APSR_nzcv_fpscr WORD $0xeef1fa10

// Uses F0, F1, F2, F3, F4, F6
TEXT ·FastAtan2(SB), NOSPLIT, $-4
	MOVF y+0(FP), F6
	MOVF x+4(FP), F4

	ABSF F6, F2

	MOVF $1e-20, F0
	ADDF F0, F2

	WORD $0xeeb54ac0   // vcmpe.f32 s8, #0x0
	vmrs_APSR_nzcv_fpscr
	BGT  fatan2_pos_x
	BEQ  fatan2_zero_x

	ADDF F2, F4, F1             // x + abs(y)
	SUBF F4, F2, F4             // abs(y) - x
	MOVF $2.356194496154785, F3 // pi * 3/4
	B    fatan2_2

fatan2_pos_x:
	SUBF F2, F4, F1              // x - abs(y)
	ADDF F2, F4, F4              // abs(y) + x
	MOVF $0.7853981852531433, F3 // pi * 1/4

fatan2_2:
	DIVF F4, F1, F2

	MOVF $0.1963, F1
	MULF F2, F1
	MULF F2, F1
	MOVF $0.9817, F0
	SUBF F0, F1
	MULF F2, F1
	ADDF F3, F1

	WORD $0xeeb56ac0   // vcmpe.f32 s12, #0x0
	vmrs_APSR_nzcv_fpscr
	WORD $0xbeb11a41   // vneglt.f32 s2, s2
	MOVF F1, ret+8(FP)
	RET

fatan2_zero_x:
	WORD $0xeeb56ac0   // vcmpe.f32 s12, #0x0
	vmrs_APSR_nzcv_fpscr
	BGT  fatan2_pos_y
	BLT  fatan2_neg_y
	MOVF F6, res+8(FP)
	RET

fatan2_neg_y:
	MOVF negativeHalfPi, F6
	MOVF F6, res+8(FP)
	RET

fatan2_pos_y:
	MOVF halfPi, F6
	MOVF F6, res+8(FP)
	RET

// Uses F0, F1, F2, F3, F4, F6
TEXT ·FastAtan2_2(SB), NOSPLIT, $-4
	MOVF x+4(FP), F6
	MOVF y+0(FP), F3
	WORD $0xeeb56ac0    // vcmpe.f32 s12, #0x0
	vmrs_APSR_nzcv_fpscr
	BEQ  fatan22_zero_x

	// y / x
	DIVF F6, F3, F1
	MULF F1, F1, F2
	MOVF $1.0, F0

	// CMPF F0, F2
	WORD $0xeeb42ac0 // vcmpe.f32 s4, s0
	vmrs_APSR_nzcv_fpscr
	BGT  fatan22_5

	// z / (1.0 + 0.28*z*z)
	MOVF    $0.28, F4
	MULF    F4, F2
	ADDF    F0, F2
	DIVF    F2, F1, F2
	WORD    $0xeeb56ac0 // vcmpe.f32 s12, #0x0
	vmrs_APSR_nzcv_fpscr
	BGE     fatan22_6
	MOVF    pi, F1
	WORD    $0xeeb53ac0 // vcmpe.f32 s6, #0x0
	vmrs_APSR_nzcv_fpscr
	SUBF.LT F1, F2
	ADDF.GE F1, F2

fatan22_6:
	MOVF F2, res+8(FP)
	RET

fatan22_5:
	// pi2 - z/(z*z+0.28)
	MOVF    $0.28, F4
	ADDF    F4, F2
	DIVF    F2, F1, F2
	MOVF    halfPi, F1
	SUBF    F2, F1, F2
	MOVF    pi, F1
	WORD    $0xeeb53ac0   // vcmpe.f32 s6, #0x0
	vmrs_APSR_nzcv_fpscr
	SUBF.LT F1, F2
	MOVF    F2, res+8(FP)
	RET

fatan22_zero_x:
	WORD $0xeeb53ac0 // vcmpe.f32 s6, #0x0
	vmrs_APSR_nzcv_fpscr

	// MOVF.LT	negativeHalfPi, F6
	// MOVF.GT	halfPi, F6
	// MOVF	F6, res+8(FP)
	// RET
	BGT  fatan22_pi2
	BLT  fatan22_neg_pi2
	MOVF F6, res+8(FP)
	RET

fatan22_neg_pi2:
	MOVF negativeHalfPi, F6
	MOVF F6, res+8(FP)
	RET

fatan22_pi2:
	MOVF halfPi, F6
	MOVF F6, res+8(FP)
	RET

TEXT ·VScaleF32(SB), NOSPLIT, $0
	B ·vscaleF32(SB)

TEXT ·VMulC64xF32(SB), NOSPLIT, $0
	B ·vMulC64xF32(SB)

TEXT ·VAbsC64(SB), NOSPLIT, $0
	MOVW input+0(FP), R0
	MOVW output+12(FP), R1
	MOVW input_len+4(FP), R2
	MOVW output_len+16(FP), R3

	// Choose the shortest length
	CMP     R2, R3
	MOVW.LT R3, R2

	// If no input then skip loop
	CMP $0, R2
	BEQ vabsc64_done

	MOVBU ·UseVector+0(SB), R3
	TEQ   $0, R3
	BEQ   vabsc64_scalar_loop

	CMP $4, R2
	BLT vabsc64_scalar_loop

	PLD (R0)
	PLD 64(R0)
	PLD (2*64)(R0)
	PLD (3*64)(R0)

	// Set vector length to 4 and stride to 2
	WORD $0xeef13a10            // vmrs r3, fpscr
	BIC  $((7<<16)|(3<<20)), R3
	ORR  $((3<<16)|(1<<20)), R3
	WORD $0xeee13a10            // fmxr fpscr, r3

vabsc64_vector_loop:
	PLD (4*64)(R0)

	WORD $0xecb04a08 // vldmia r0!, {s8-s15}
	WORD $0xee244a04 // vmul.f32 s8, s8, s8
	WORD $0xee044aa4 // vmla.f32 s8, s9, s9
	WORD $0xeeb14ac4 // vsqrt.f32 s8, s8
	WORD $0xed814a00 // vstr s8, [r1]
	WORD $0xed815a01 // vstr s10, [r1, #0x4]
	WORD $0xed816a02 // vstr s12, [r1, #0x8]
	WORD $0xed817a03 // vstr s14, [r1, #0xc]
	ADD  $16, R1

	SUB $4, R2
	CMP $4, R2
	BGE vabsc64_vector_loop

	// Clear vector mode
	WORD $0xeef13a10            // vmrs r3, fpscr
	BIC  $((7<<16)|(3<<20)), R3
	WORD $0xeee13a10            // fmxr fpscr, r3

	TEQ $0, R2
	BEQ vabsc64_done

vabsc64_scalar_loop:
	MOVF  0(R0), F0           // real
	MOVF  4(R0), F1           // imag
	ADD   $8, R0
	MULF  F0, F0
	MULF  F1, F1
	ADDF  F1, F0
	SQRTF F0, F0
	MOVF  F0, 0(R1)
	ADD   $4, R1
	SUB   $1, R2
	TEQ   $0, R2
	BNE   vabsc64_scalar_loop

vabsc64_done:
	RET

TEXT ·VMaxF32(SB), 7, $0
	MOVW input+0(FP), R0
	MOVW input_len+4(FP), R2

	MOVW $0xff800000, R1
	MOVW R1, F4

	CMP $0, R2
	BEQ vmaxf32_done

	CMP $4, R2
	BLT vmaxf32_scalar_loop

	PLD (R0)
	PLD 64(R0)
	PLD (2*64)(R0)

vmaxf32_batch_loop:
	PLD  (3*64)(R0)
	WORD $0xecb00a04        // vldmia r0!, {s0-s3}
	WORD $0xeeb40ac4        // vcmpe.f32 s0, s8
	vmrs_APSR_nzcv_fpscr
	WORD $0xceb04a40        // vmovgt.f32 s8, s0
	WORD $0xeef40ac4        // vcmpe.f32 s1, s8
	vmrs_APSR_nzcv_fpscr
	WORD $0xceb04a60        // vmovgt.f32 s8, s1
	WORD $0xeeb41ac4        // vcmpe.f32 s2, s8
	vmrs_APSR_nzcv_fpscr
	WORD $0xceb04a41        // vmovgt.f32 s8, s2
	WORD $0xeef41ac4        // vcmpe.f32 s3, s8
	vmrs_APSR_nzcv_fpscr
	WORD $0xceb04a61        // vmovgt.f32 s8, s3
	SUB  $4, R2
	CMP  $4, R2
	BGE  vmaxf32_batch_loop

	TEQ $0, R2
	BEQ vmaxf32_done

vmaxf32_scalar_loop:
	MOVF 0(R0), F1
	ADD  $4, R0

	// CMPF    F4, F1
	WORD    $0xeeb41ac4         // vcmpe.f32 s2, s8
	vmrs_APSR_nzcv_fpscr
	MOVF.GT F1, F4
	SUB     $1, R2
	TEQ     $0, R2
	BNE     vmaxf32_scalar_loop

vmaxf32_done:
	MOVF F4, res+12(FP)
	RET
