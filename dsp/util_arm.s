#include "textflag.h"

TEXT ·rotate90FilterAsm(SB), NOSPLIT, $0
	MOVW samples_len+8(FP), R7
	MOVW samples_ptr+4(FP), R8
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
	MOVW samples_ptr+4(FP), R0
	MOVW R0, ret_ptr+16(FP)
	MOVW samples_len+8(FP), R0
	MOVW R0, ret_len+20(FP)
	MOVW samples_cap+12(FP), R0
	MOVW R0, ret_cap+24(FP)
	RET

TEXT ·i32Rotate90FilterAsm(SB), NOSPLIT, $0
	MOVW samples_len+8(FP), R7
	MOVW samples_ptr+4(FP), R8
	AND  $(~3), R7             // round down to nearest multiple of 4

	TEQ $0, R7
	BEQ i32r90_end

	ADD R7<<2, R8, R7

i32r90_loop:
	// First sample of the group of 4 doesn't change
	ADD $8, R8

	MOVM.IA (R8), [R0-R5]

	// samples[i+1] = complex(-imag(samples[i+1]), real(samples[i+1]))
	MOVW R0, R6
	MVN  R1, R0
	MOVW R6, R1

	// samples[i+2] = -samples[i+2]
	MVN R2, R2
	MVN R3, R3

	// samples[i+3] = complex(imag(samples[i+3]), -real(samples[i+3]))
	MVN       R4, R6
	MOVW      R5, R4
	MOVW      R6, R5
	MOVM.IA.W [R0-R5], (R8)

	CMP R8, R7
	BGT i32r90_loop

i32r90_end:
	MOVW samples_ptr+4(FP), R0
	MOVW R0, ret_ptr+16(FP)
	MOVW samples_len+8(FP), R0
	MOVW R0, ret_len+20(FP)
	MOVW samples_cap+12(FP), R0
	MOVW R0, ret_cap+24(FP)
	RET
