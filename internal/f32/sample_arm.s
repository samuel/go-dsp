// VFP kernels for the per-sample loops in sample.go, which define their
// behavior. Every value is rounded where the reference rounds it, so the two
// agree bit for bit.

#include "textflag.h"

#define pi $3.14159265358979323846264338327950288419716939937510582097494459
#define halfPi $1.570796326794896557998981734272092580795288085938

#define vmrs_APSR_nzcv_fpscr WORD $0xeef1fa10

// F1 = atanPoly(F0), using F5 and F7.
#define ATAN_POLY \
	MULF F0, F0, F5;         \
	MOVF $-0.0443266146, F1; \
	MULF F5, F1;             \
	MOVF $0.155578747, F7;   \
	ADDF F7, F1;             \
	MULF F5, F1;             \
	MOVF $-0.325808436, F7;  \
	ADDF F7, F1;             \
	MULF F5, F1;             \
	MOVF $0.999787867, F7;   \
	ADDF F7, F1;             \
	MULF F0, F1

// F1 = FastAtan2(y=F6, x=F4), using F0, F2, F3, F5 and F7, continuing at done.
// The labels are arguments so that each function has its own.
//
// F2 and F3 hold |y| and |x|. The first compare (vcmpe.f32 s4, s6) is false
// for a NaN as well as for |y| <= |x|, and the second (vcmpe.f32 s6, #0) then
// sends a zero or NaN |x| to F2+F3, which is +0 or NaN as in the reference.
// Unfolding compares x (s8) and y (s12) with zero and acts on MI rather than
// LT, which an unordered compare also meets: vnegmi.f32 s2, s2 negates the
// result for a negative y.
#define ATAN2(swap, fold, unfold, done) \
	ABSF F6, F2;            \
	ABSF F4, F3;            \
	WORD $0xeeb42ac3;       \
	vmrs_APSR_nzcv_fpscr;   \
	BGT  swap;              \
	WORD $0xeeb53ac0;       \
	vmrs_APSR_nzcv_fpscr;   \
	BGT  fold;              \
	ADDF F2, F3, F1;        \
	B    done;              \
fold:                       \
	DIVF F3, F2, F0;        \
	ATAN_POLY;              \
	B    unfold;            \
swap:                       \
	DIVF F2, F3, F0;        \
	ATAN_POLY;              \
	MOVF halfPi, F0;        \
	SUBF F1, F0, F1;        \
unfold:                     \
	WORD $0xeeb54ac0;       \
	vmrs_APSR_nzcv_fpscr;   \
	MOVF    pi, F0;         \
	SUBF.MI F1, F0, F1;     \
	WORD $0xeeb56ac0;       \
	vmrs_APSR_nzcv_fpscr;   \
	WORD $0x4eb11a41;       \
done:

TEXT ·FastAtan2(SB), NOSPLIT, $-4
	MOVF y+0(FP), F6
	MOVF x+4(FP), F4
	ATAN2(atan_swap, atan_fold, atan_unfold, atan_done)
	MOVF F1, ret+8(FP)
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
	MOVF pre_real+24(FP), F8
	MOVF pre_imag+28(FP), F9

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

	MULF F3, F8, F6 // imag(inp)*real(pre)
	MULF F2, F9, F0 // real(inp)*imag(pre)
	MULF F2, F8, F4 // real(inp)*real(pre)
	MULF F3, F9, F7 // imag(inp)*imag(pre)
	SUBF F0, F6
	ADDF F7, F4

	MOVF F2, F8 // real(pre) = real(inp)
	MOVF F3, F9 // imag(pre) = imag(inp)

	ATAN2(fmDemod_swap, fmDemod_fold, fmDemod_unfold, fmDemod_atan_done)
	MOVF F1, 0(R3)
	ADD $4, R3

	SUB $1, R2
	TEQ $0, R2
	BNE fmDemod_loop

fmDemod_done:
	MOVF F8, ret_real+32(FP)
	MOVF F9, ret_imag+36(FP)
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
