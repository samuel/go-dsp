#include "textflag.h"

#define pi $3.14159265358979323846264338327950288419716939937510582097494459
#define halfPi $1.570796326794896557998981734272092580795288085938
#define negativeHalfPi $-1.570796326794896557998981734272092580795288085938

#define vmrs_APSR_nzcv_fpscr WORD $0xeef1fa10

// Uses F0, F1, F2, F3, F4, F6
TEXT ·fastAtan2Asm(SB), NOSPLIT, $-4
	MOVF y+0(FP), F6
	MOVF x+4(FP), F4

	ABSF F6, F2

	MOVF $1e-20, F0
	ADDF F0, F2

	WORD $0xeeb54ac0   // vcmpe.f32 s8, #0x0
	vmrs_APSR_nzcv_fpscr
	BGT  fatan2_pos_x
	BEQ  fatan2_zero_x
	BVS  fatan2_zero_x // x is NaN, so the compare is unordered; the Go
	                   // reference finds it neither negative nor positive
	                   // and falls through to the y tests too

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

	// MI rather than LT: LT is also true for an unordered compare, which would
	// send a NaN y to -Pi/2 where the reference returns zero.
	BMI  fatan2_neg_y

	// y is zero, a negative zero or a NaN. The reference returns a positive
	// zero for all three, so returning y itself will not do.
	MOVF $0.0, F6
	MOVF F6, ret+8(FP)
	RET

fatan2_neg_y:
	MOVF negativeHalfPi, F6
	MOVF F6, ret+8(FP)
	RET

fatan2_pos_y:
	MOVF halfPi, F6
	MOVF F6, ret+8(FP)
	RET

// Uses F0, F1, F2, F3, F4, F6
TEXT ·fastAtan2FineAsm(SB), NOSPLIT, $-4
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

	// GE, not GT: the reference branches on zz < 1.0, so zz == 1.0 takes the
	// other arm. With GT, FastAtan2Fine(1, 1) returned 0.78125 here against
	// 0.7895464 everywhere else. GE is false for an unordered compare, which
	// sends a NaN zz down this arm -- harmless, since z is NaN there and both
	// arms then produce NaN.
	BGE  fatan22_5

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
	MOVF F2, ret+8(FP)
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
	MOVF    F2, ret+8(FP)
	RET

fatan22_zero_x:
	WORD $0xeeb53ac0 // vcmpe.f32 s6, #0x0
	vmrs_APSR_nzcv_fpscr
	BGT  fatan22_pi2

	// MI rather than LT, for the reason given in fastAtan2Asm above.
	BMI  fatan22_neg_pi2

	// F6 still holds x, which is a zero of either sign here; the reference
	// returns a positive one for it and for a NaN y.
	MOVF $0.0, F6
	MOVF F6, ret+8(FP)
	RET

fatan22_neg_pi2:
	MOVF negativeHalfPi, F6
	MOVF F6, ret+8(FP)
	RET

fatan22_pi2:
	MOVF halfPi, F6
	MOVF F6, ret+8(FP)
	RET
