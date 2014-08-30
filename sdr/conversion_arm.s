
TEXT ·Ui8toi16(SB),7,$0
	MOVW	input+0(FP), R1
	MOVW	input_len+4(FP), R2
	MOVW	output+12(FP), R3
	MOVW	output_len+16(FP), R4
	// Choose the shortest length
	CMP	R2, R4
	MOVW.LT	R4, R2
	// If no input then skip loop
	CMP	$0, R2
	BEQ	ui8toi16_done
	// Calculate end of input
	ADD	R1, R2
ui8toi16_loop:
	MOVBU	0(R1), R0
	ADD	$1, R1

	SUB	$128, R0

	MOVBU	R0, 0(R3)
	MOVBU	R0, 1(R3)
	ADD	$2, R3

	CMP	R2, R1
	BLT	ui8toi16_loop
ui8toi16_done:
	RET

TEXT ·Ui8toi16b(SB),7,$0
	MOVW	output_len+16(FP), R4
	MOVW	R4>>1, R4
	MOVW	R4, output_len+16(FP)
	B	·Ui8toi16(SB)



TEXT ·Ui8tof32(SB),7,$0
	MOVW	input+0(FP), R1
	MOVW	input_len+4(FP), R2
	MOVW	output+12(FP), R3
	MOVW	output_len+16(FP), R0
	// Choose the shortest length
	CMP	R2, R0
	MOVW.LT	R0, R2
	// If no input then skip loop
	CMP	$0, R2
	BEQ	ui8tof32_done
	ADD	R1, R2
ui8tof32_loop:
	MOVBU	0(R1), R4
	ADD	$1, R1
	SUB	$128, R4
	MOVW	R4, F0
	MOVWF	F0, F0
	MOVF	F0, 0(R3)
	ADD	$4, R3
	CMP	R2, R1
	BLT	ui8tof32_loop
ui8tof32_done:
	RET



TEXT ·Ui8toc64(SB),7,$0
	MOVW	input+0(FP), R1
	MOVW	input_len+4(FP), R2
	MOVW	output+12(FP), R3
	MOVW	output_len+16(FP), R0
	// Choose the shortest length
	AND	$-2, R2
	MOVW	R0<<1, R0
	CMP	R2, R0
	MOVW.LT	R0, R2
	// If no input then skip loop
	CMP	$0, R2
	BEQ	ui8toc64_done
	ADD	R1, R2
ui8toc64_loop:
	MOVBU	0(R1), R4	// real
	MOVBU	1(R1), R5	// imag
	ADD	$2, R1

	SUB	$128, R4
	SUB	$128, R5
	MOVW	R4, F0
	MOVWF	F0, F0
	MOVW	R5, F1
	MOVWF	F1, F1

	MOVF	F0, 0(R3)	// real
	MOVF	F1, 4(R3)	// imag
	ADD	$8, R3

	CMP	R2, R1
	BLT	ui8toc64_loop
ui8toc64_done:
	RET



TEXT ·F32toi16(SB),7,$0
	MOVW	input+0(FP), R1
	MOVW	input_len+4(FP), R2
	MOVW	output+12(FP), R3
	MOVW	output_len+16(FP), R0
	MOVF	scale+24(FP), F0
	// Choose the shortest length
	CMP	R2, R0
	MOVW.LT	R0, R2
	MOVW	R2<<2, R2
	// If no input then skip loop
	CMP	$0, R2
	BEQ	f32toi16_done
	ADD	R1, R2
f32toi16_loop:
	MOVF	0(R1), F1
	ADD	$4, R1
	MULF	F0, F1
	MOVFW	F1, F1
	MOVW	F1, R0
	MOVHU	R0, (R3)
	ADD	$2, R3
	CMP	R2, R1
	BLT	f32toi16_loop
f32toi16_done:
	RET



// TODO: detect endianess and use faster single instruction if native little-endian
TEXT ·F32toi16ble(SB),7,$0
	MOVW	input+0(FP), R1
	MOVW	input_len+4(FP), R2
	MOVW	output+12(FP), R3
	MOVW	output_len+16(FP), R0
	MOVF	scale+24(FP), F0
	// Choose the shortest length
	MOVW	R2<<1, R2
	CMP	R2, R0
	MOVW.LT	R0, R2
	MOVW	R2<<1, R2
	// If no input then skip loop
	CMP	$0, R2
	BEQ	f32toi16b_done
	ADD	R1, R2
f32toi16b_loop:
	MOVF	0(R1), F1
	ADD	$4, R1

	MULF	F0, F1
	MOVFW	F1, F1
	MOVW	F1, R0

	// Native endianess
	MOVHU	R0, (R3)

	// Little endian
	// MOVW	R0>>8, R4
	// MOVBU	R0, 0(R3)
	// MOVBU	R4, 1(R3)

	ADD	$2, R3

	CMP	R2, R1
	BLT	f32toi16b_loop
f32toi16b_done:
	RET
