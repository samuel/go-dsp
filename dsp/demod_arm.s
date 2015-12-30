#include "textflag.h"

TEXT ·fmDemodulateAsm(SB), NOSPLIT, $0
	MOVW input+4(FP), R1
	MOVW input_len+8(FP), R2
	MOVW output+16(FP), R3
	MOVW output_len+20(FP), R4

	// Choose the shortest length
	CMP     R2, R4
	MOVW.LT R4, R2

	// If no input then skip loop
	TEQ $0, R2
	BEQ fmDemod_done

	MOVW fi+0(FP), R0
	MOVF 0(R0), F5    // real(pre)
	MOVF 4(R0), F1    // imag(pre)

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
	WORD    $0xeeb56ac0                                              // vcmpe.f32 s12, #0x0
	WORD    $0xeef1fa10                                              // vmrs APSR_nzcv, fpscr
	MOVF.LT $-1.570796326794896557998981734272092580795288085938, F6
	MOVF.GT $1.570796326794896557998981734272092580795288085938, F6
	MOVF    F6, 0(R3)

fmDemod_atan_done:

	//

	ADD $4, R3

	SUB $1, R2
	TEQ $0, R2
	BNE fmDemod_loop

	MOVF F5, 0(R0) // real(pre)
	MOVF F1, 4(R0) // imag(pre)

fmDemod_done:
	MOVW input_len+8(FP), R0
	MOVW R0, output_len+28(FP)
	RET
