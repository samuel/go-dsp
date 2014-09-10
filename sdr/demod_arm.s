
TEXT ·fmDemodulateAsm(SB),7,$0
	MOVW	input+4(FP), R1
	MOVW	input_len+8(FP), R2
	MOVW	output+16(FP), R3
	MOVW	output_len+20(FP), R4

	// Choose the shortest length
	CMP	R2, R4
	MOVW.LT	R4, R2
	// If no input then skip loop
	TEQ	$0, R2
	BEQ	fmDemod_done

	MOVW	fi+0(FP), R0
	MOVF	0(R0), F0 // real(pre)
	MOVF	4(R0), F1 // imag(pre)

fmDemod_loop:
	MOVF	0(R0), F0 // real(pre)
	MOVF	4(R0), F1 // imag(pre)
	MOVF	0(R1), F2 // real(inp)
	MOVF	4(R1), F3 // imag(inp)
	ADD	$8, R1
	MOVF	F2, 0(R0) // real(pre)
	MOVF	F3, 4(R0) // imag(pre)

	MULF	F3, F0, F6 // imag(inp)*real(pre)
	MULF	F2, F1, F5 // real(inp)*imag(pre)
	SUBF	F5, F6
	MULF	F2, F0, F4 // real(inp)*real(pre)
	MULF	F3, F1, F7 // imag(inp)*imag(pre)
	ADDF	F7, F4

	// MOVF	F2, F0
	// MOVF	F3, F1

	// MOVF	F4, -12(SP)
	// MOVF	F6, -8(SP)
	// BL	·FastAtan2(SB)
	// MOVF	-4(SP), F2

	// FastAtan2

	ABSF	F6, F2

	MOVF	$1e-10, F0
	ADDF	F0, F2

	MOVF	$0.0, F5
	CMPF	F5, F4
	BGE	L1

	ADDF	F2, F4, F7	// x + abs(y)
	SUBF	F4, F2, F4	// abs(y) - x
	MOVF	$2.356194496154785, F3	// pi * 3/4
	B	L2
L1:
	SUBF	F2, F4, F7	// x - abs(y)
	ADDF	F2, F4, F4	// abs(y) + x
	MOVF	$0.7853981852531433, F3	// pi * 1/4
L2:
	DIVF	F4, F7, F2

	MOVF	$0.1963, F7
	MULF	F2, F7
	MULF	F2, F7
	MOVF	$0.9817, F0
	SUBF	F0, F7
	MULF	F2, F7
	ADDF	F3, F7

	MOVF	$-1.0, F0
	CMPF	F5, F6
	MULF.LT	F0, F7

	//

	MOVF	F7, 0(R3)
	ADD	$4, R3

	SUB	$1, R2
	TEQ	$0, R2
	BNE    	fmDemod_loop

	// MOVW	fi+0(FP), R0
	MOVF	F0, 0(R0) // real(pre)
	MOVF	F1, 4(R0) // imag(pre)

fmDemod_done:
	MOVW   	input_len+8(FP), R0
	MOVW   	R0,output_len+28(FP)
	MOVW   	$0, R0
	MOVW   	R0, err+32(FP)
	MOVW   	R0, err+36(FP)
	RET

// TEXT ·fmDemodulateAsm(SB),7,$12-36
// 	MOVW	input+4(FP), R1
// 	MOVW	input_len+8(FP), R2
// 	MOVW	output+16(FP), R3
// 	MOVW	output_len+20(FP), R4

// 	// Choose the shortest length
// 	CMP	R2, R4
// 	MOVW.LT	R4, R2
// 	// If no input then skip loop
// 	TEQ	$0, R2
// 	BEQ	fmDemod_done

// 	MOVW	fi+0(FP), R0

// 	MOVF	0(R0), F0 // real(pre)
// 	MOVF	4(R0), F1 // imag(pre)
// fmDemod_loop:
// 	MOVF	0(R1), F2 // real(inp)
// 	MOVF	4(R1), F3 // imag(inp)

// 	MULF	F3, F0, F6 // imag(inp)*real(pre)
// 	MULF	F2, F1, F5 // real(inp)*imag(pre)
// 	SUBF	F5, F6
// 	MULF	F2, F0, F4 // real(inp)*real(pre)
// 	MULF	F3, F1, F7 // imag(inp)*imag(pre)
// 	ADDF	F7, F4

// 	MOVF	F6, -12(SP)
// 	MOVF	F4, -8(SP)
// 	BL	·FastAtan2(SB)
// 	MOVF	-4(SP), F2

// 	MOVF	F2, 0(R3)
// 	ADD	$4, R3

// 	MOVF	0(R1), F0 // real(inp)
// 	MOVF	4(R1), F1 // imag(inp)
// 	ADD	$8, R1

// 	SUB	$1, R2
// 	TEQ	$0, R2
// 	BNE    	fmDemod_loop

// fmDemod_done:
// 	MOVF	F0, 0(R0)
// 	MOVF	F1, 4(R0)

// 	MOVW   	input_len+8(FP), R0
// 	MOVW   	R0,output_len+28(FP)
// 	MOVW   	$0, R0
// 	MOVW   	R0, err+32(FP)
// 	MOVW   	R0, err+36(FP)
// 	RET
