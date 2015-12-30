#include "textflag.h"

TEXT ·lowPassDownsampleComplexFilterAsm(SB), NOSPLIT, $0
	MOVW fi+0(FP), R3
	MOVW 0(R3), R8              // fi.Downsample
	MOVW 12(R3), R7             // fi.prevIndex
	MOVW samples_len+8(FP), R2
	MOVW samples_data+4(FP), R5 // input
	MOVW R5, R6                 // output
	MOVF 4(R3), F0              // real(fi.now)
	MOVF 8(R3), F1              // imag(fi.now)
	B    complexLoopStart

complexLoop:
	SUB $1, R2

complexLoopStart:
	TEQ $0, R2
	BEQ complexLoopEnd

	// samples[i]
	MOVF 0(R5), F2 // real
	MOVF 4(R5), F3 // imag
	ADD  $8, R5

	// fi.now += samples[i]
	ADDF F2, F0
	ADDF F3, F1

	// fi.prevIndex++
	ADD $1, R7

	// if prevIndex < downsample: continue
	CMP R8, R7
	BLT complexLoop

	// samples[i2] = fi.now
	MOVF F0, 0(R6)
	MOVF F1, 4(R6)
	ADD  $8, R6

	// fi.prevIndex = 0
	MOVW $0, R7

	// fi.now = 0.0
	MOVF $0.0, F0
	MOVF $0.0, F1

	B complexLoop

complexLoopEnd:
	MOVW R7, 12(R3) // fi.prevIndex
	MOVF F0, 4(R3)  // real(fi.now)
	MOVF F1, 8(R3)  // imag(fi.now)

	MOVW samples_data+4(FP), R0
	SUB  R0, R6
	MOVW R6>>3, R6
	MOVW R6, ret_len+20(FP)
	MOVW samples_cap+12(FP), R4
	MOVW R4, ret_cap+24(FP)
	MOVW samples_data+4(FP), R0
	MOVW R0, ret_data+16(FP)
	RET

TEXT ·lowPassDownsampleRationalFilterAsm(SB), NOSPLIT, $0
	MOVW fi+0(FP), R4 // fi

	MOVW  4(R4), R7 // fi.Slow
	MOVW  R7, F4
	MOVWF F4, F4

	MOVW  0(R4), R8 // fi.Fast
	MOVW  R8, F3
	MOVWF F3, F3

	DIVF F3, F4 // fi.Slow / fi.Fast

	MOVF 8(R4), F3  // fi.sum
	MOVW 12(R4), R2 // fi.prevIndex

	MOVW samples_ptr+4(FP), R5 // input
	MOVW R5, R6                // output
	MOVW samples_len+8(FP), R3
	ADD  R3<<2, R5, R3         // end of input

rationalLoop:
	CMP R5, R3
	BLE rationalLoopEnd

	MOVF (R5), F0 // samples[i]
	ADD  $4, R5

	ADDF F0, F3 // fi.sum += samples[i]
	ADD  R7, R2 // fi.prevIndex += fi.Slow

	CMP R8, R2
	BLT rationalLoop

	MULF F4, F3 // fi.sum * (Slow/Fast)

	MOVF F3, (R6)
	ADD  $4, R6

	SUB  R8, R2   // fi.prevIndex -= fi.Fast
	MOVF $0.0, F3 // fi.sum = 0.0

	B rationalLoop

rationalLoopEnd:
	MOVW R2, 12(R4) // fi.prevIndex
	MOVF F3, 8(R4)  // fi.sum

	MOVW samples_ptr+4(FP), R0
	SUB  R0, R6
	MOVW R6>>2, R6
	MOVW R6, res_len+20(FP)
	MOVW samples_cap+12(FP), R4
	MOVW R4, res_cap+24(FP)
	MOVW samples_ptr+4(FP), R0
	MOVW R0, res_ptr+16(FP)
	RET
