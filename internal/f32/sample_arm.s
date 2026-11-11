// VFP kernels for the per-sample loops in sample.go, which define their
// behavior. Every value is rounded where the reference rounds it, so the two
// agree bit for bit.

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
TEXT ·FastAtan2Fine(SB), NOSPLIT, $-4
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

	// MI rather than LT, for the reason given in FastAtan2 above.
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

TEXT ·Rotate90(SB), NOSPLIT, $0
	MOVW samples_len+4(FP), R7
	MOVW samples_base+0(FP), R8
	AND  $(~3), R7             // round down to nearest multiple of 4

	TEQ $0, R7
	BEQ r90_end

	ADD R7<<3, R8, R7

r90_loop:
	// First sample of the group of 4 doesn't change
	ADD $8, R8

	MOVM.IA (R8), [R0-R5]

	// samples[i+1] = complex(-imag(samples[i+1]), real(samples[i+1]))
	MOVW R0, R6
	EOR  $(1<<31), R1, R0
	MOVW R6, R1

	// samples[i+2] = -samples[i+2]
	EOR $(1<<31), R2
	EOR $(1<<31), R3

	// samples[i+3] = complex(imag(samples[i+3]), -real(samples[i+3]))
	EOR       $(1<<31), R4, R6
	MOVW      R5, R4
	MOVW      R6, R5
	MOVM.IA.W [R0-R5], (R8)

	CMP R8, R7
	BGT r90_loop

r90_end:
	RET

TEXT ·FMDemod(SB), NOSPLIT, $0
	MOVW src_base+12(FP), R1
	MOVW src_len+16(FP), R2
	MOVW dst_base+0(FP), R3
	MOVW dst_len+4(FP), R4
	MOVF pre_real+24(FP), F5
	MOVF pre_imag+28(FP), F1

	// Choose the shortest length
	CMP     R2, R4
	MOVW.LT R4, R2

	// If no input then skip loop
	TEQ $0, R2
	BEQ fmDemod_done

fmDemod_loop:
	MOVF 0(R1), F2 // real(inp)
	MOVF 4(R1), F3 // imag(inp)
	ADD  $8, R1

	MULF F3, F5, F6 // imag(inp)*real(pre)
	MULF F2, F1, F0 // real(inp)*imag(pre)
	MULF F2, F5, F4 // real(inp)*real(pre)
	MULF F3, F1, F7 // imag(inp)*imag(pre)
	SUBF F0, F6
	ADDF F7, F4

	MOVF F2, F5 // real(pre) = real(inp)
	MOVF F3, F1 // imag(pre) = imag(inp)

	// FastAtan2(y=F6, x=F4)

	ABSF F6, F2
	MOVF $1e-20, F0
	ADDF F0, F2
	WORD $0xeeb54ac0            // vcmpe.f32 s8, #0x0
	WORD $0xeef1fa10            // vmrs APSR_nzcv, fpscr
	BEQ  fmDemod_atan_zero_x
	BGT  fmDemod_atan_pos_x
	BVS  fmDemod_atan_zero_x    // x is NaN: unordered, and the Go reference
	                            // falls through to the y tests for it too
	ADDF F2, F4, F7             // x + abs(y)
	SUBF F4, F2, F4             // abs(y) - x
	MOVF $2.356194496154785, F3 // pi * 3/4
	B    fmDemod_atan_1

fmDemod_atan_pos_x:
	SUBF F2, F4, F7              // x - abs(y)
	ADDF F2, F4, F4              // abs(y) + x
	MOVF $0.7853981852531433, F3 // pi * 1/4

fmDemod_atan_1:
	DIVF F4, F7, F2
	MOVF $0.1963, F7
	MULF F2, F7
	MULF F2, F7
	MOVF $0.9817, F0
	SUBF F0, F7
	MULF F2, F7
	ADDF F3, F7
	WORD $0xeeb56ac0       // vcmpe.f32 s12, #0x0
	WORD $0xeef1fa10       // vmrs APSR_nzcv, fpscr
	WORD $0xbeb17a47       // vneglt.f32 s14, s14
	MOVF F7, 0(R3)
	B    fmDemod_atan_done

fmDemod_atan_zero_x:
	WORD $0xeeb56ac0 // vcmpe.f32 s12, #0x0
	WORD $0xeef1fa10 // vmrs APSR_nzcv, fpscr

	// LE covers equal, less-than and unordered, so a zero, a negative zero or
	// a NaN y all start at +0 as the Go reference returns; MI then claims the
	// strictly negative case, which LT would have shared with a NaN.
	MOVF.LE $0.0, F6
	MOVF.MI $-1.570796326794896557998981734272092580795288085938, F6
	MOVF.GT $1.570796326794896557998981734272092580795288085938, F6
	MOVF    F6, 0(R3)

fmDemod_atan_done:

	//

	ADD $4, R3

	SUB $1, R2
	TEQ $0, R2
	BNE fmDemod_loop

fmDemod_done:
	MOVF F5, ret_real+32(FP)
	MOVF F1, ret_imag+36(FP)
	RET

TEXT ·BoxcarDecimate(SB), NOSPLIT, $0
	MOVW factor+24(FP), R8
	MOVW count+36(FP), R7
	MOVW src_len+16(FP), R2
	MOVW src_base+12(FP), R5    // input
	MOVW dst_base+0(FP), R6     // output
	MOVF sum_real+28(FP), F0
	MOVF sum_imag+32(FP), F1
	B    complexLoopStart

complexLoop:
	SUB $1, R2

complexLoopStart:
	TEQ $0, R2
	BEQ complexLoopEnd

	// src[i]
	MOVF 0(R5), F2 // real
	MOVF 4(R5), F3 // imag
	ADD  $8, R5

	// sum += src[i]
	ADDF F2, F0
	ADDF F3, F1

	// count++
	ADD $1, R7

	// if count < factor: continue
	CMP R8, R7
	BLT complexLoop

	// dst[n] = sum
	MOVF F0, 0(R6)
	MOVF F1, 4(R6)
	ADD  $8, R6

	// count = 0
	MOVW $0, R7

	// sum = 0
	MOVF $0.0, F0
	MOVF $0.0, F1

	B complexLoop

complexLoopEnd:
	MOVW R7, ret2+52(FP)
	MOVF F0, ret1_real+44(FP)
	MOVF F1, ret1_imag+48(FP)

	MOVW dst_base+0(FP), R0
	SUB  R0, R6
	MOVW R6>>3, R6
	MOVW R6, ret+40(FP)
	RET

TEXT ·RationalBoxcarDecimate(SB), NOSPLIT, $0
	MOVW fast+24(FP), R8
	MOVW slow+28(FP), R7
	MOVF sum+32(FP), F3
	MOVW count+36(FP), R1
	MOVW phase+40(FP), R2

	MOVW src_base+12(FP), R5 // input
	MOVW dst_base+0(FP), R6  // output
	MOVW src_len+16(FP), R3
	ADD  R3<<2, R5, R3       // end of input

rationalLoop:
	CMP R5, R3
	BLE rationalLoopEnd

	MOVF (R5), F0 // src[i]
	ADD  $4, R5

	ADDF F0, F3 // sum += src[i]
	ADD  $1, R1 // count++
	ADD  R7, R2 // phase += slow

	CMP R8, R2
	BLT rationalLoop

	// dst[n] = sum / float32(count). Dividing by the count this
	// output actually accumulated rather than by a fixed slow/fast is what
	// keeps a non-integer ratio free of gain ripple.
	MOVW  R1, F1
	MOVWF F1, F1
	DIVF  F1, F3

	MOVF F3, (R6)
	ADD  $4, R6

	SUB  R8, R2   // phase -= fast
	MOVW $0, R1   // count = 0
	MOVF $0.0, F3 // sum = 0

	B rationalLoop

rationalLoopEnd:
	MOVF F3, ret1+48(FP)
	MOVW R1, ret2+52(FP)
	MOVW R2, ret3+56(FP)

	MOVW dst_base+0(FP), R0
	SUB  R0, R6
	MOVW R6>>2, R6
	MOVW R6, ret+44(FP)
	RET
