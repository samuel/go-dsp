#include "textflag.h"

TEXT ·boxcarDecimateAsm(SB), NOSPLIT, $0
	MOVW f+0(FP), R3
	MOVW 0(R3), R8              // f.downsample
	MOVW 12(R3), R7             // f.prevIndex
	MOVW src_len+20(FP), R2
	MOVW src_base+16(FP), R5    // input
	MOVW dst_base+4(FP), R6     // output
	MOVF 4(R3), F0              // real(f.now)
	MOVF 8(R3), F1              // imag(f.now)
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

	// f.now += src[i]
	ADDF F2, F0
	ADDF F3, F1

	// f.prevIndex++
	ADD $1, R7

	// if prevIndex < downsample: continue
	CMP R8, R7
	BLT complexLoop

	// dst[n] = f.now
	MOVF F0, 0(R6)
	MOVF F1, 4(R6)
	ADD  $8, R6

	// f.prevIndex = 0
	MOVW $0, R7

	// f.now = 0.0
	MOVF $0.0, F0
	MOVF $0.0, F1

	B complexLoop

complexLoopEnd:
	MOVW R7, 12(R3) // f.prevIndex
	MOVF F0, 4(R3)  // real(f.now)
	MOVF F1, 8(R3)  // imag(f.now)

	MOVW dst_base+4(FP), R0
	SUB  R0, R6
	MOVW R6>>3, R6
	MOVW R6, ret+28(FP)
	RET

TEXT ·rationalBoxcarDecimateAsm(SB), NOSPLIT, $0
	MOVW f+0(FP), R4 // f

	MOVW 0(R4), R8  // f.fast
	MOVW 4(R4), R7  // f.slow
	MOVF 8(R4), F3  // f.sum
	MOVW 12(R4), R1 // f.count
	MOVW 16(R4), R2 // f.prevIndex

	MOVW src_base+16(FP), R5 // input
	MOVW dst_base+4(FP), R6  // output
	MOVW src_len+20(FP), R3
	ADD  R3<<2, R5, R3       // end of input

rationalLoop:
	CMP R5, R3
	BLE rationalLoopEnd

	MOVF (R5), F0 // src[i]
	ADD  $4, R5

	ADDF F0, F3 // f.sum += src[i]
	ADD  $1, R1 // f.count++
	ADD  R7, R2 // f.prevIndex += f.slow

	CMP R8, R2
	BLT rationalLoop

	// dst[n] = f.sum / float32(f.count). Dividing by the count this
	// output actually accumulated rather than by a fixed slow/fast is what
	// keeps a non-integer ratio free of gain ripple.
	MOVW  R1, F1
	MOVWF F1, F1
	DIVF  F1, F3

	MOVF F3, (R6)
	ADD  $4, R6

	SUB  R8, R2   // f.prevIndex -= f.fast
	MOVW $0, R1   // f.count = 0
	MOVF $0.0, F3 // f.sum = 0.0

	B rationalLoop

rationalLoopEnd:
	MOVF F3, 8(R4)  // f.sum
	MOVW R1, 12(R4) // f.count
	MOVW R2, 16(R4) // f.prevIndex

	MOVW dst_base+4(FP), R0
	SUB  R0, R6
	MOVW R6>>2, R6
	MOVW R6, ret+28(FP)
	RET
