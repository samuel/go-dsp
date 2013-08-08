
TEXT ·rotate90FilterAsm(SB),7,$0
	MOVW	samples_len+8(FP), R7
	MOVW	samples_ptr+4(FP), R8
	AND	$(~3), R7	// round down to nearest multiple of 4
	ADD	$8, R8
	MOVW	$0, R6
	B	loopStart
loop:
	ADD	$4, R6
loopStart:
	CMP	R6, R7
	BLE	loopEnd

	MOVM.IA.W	(R8), [R0, R1, R2, R3, R4, R5]
	EOR	$(1<<31), R1
	EOR	$(1<<31), R2
	EOR	$(1<<31), R3
	EOR	$(1<<31), R4
	MOVM.DA.W	[R1, R0, R2, R3, R5, R4], (R8)
	ADD	$32, R8

	// MOVW	8(R8), R0
	// MOVW	12(R8), R1
	// EOR	$(1<<31), R1
	// MOVW	R1, 8(R8)
	// MOVW	R0, 12(R8)

	// MOVW	16(R8), R0
	// MOVW	20(R8), R1
	// EOR	$(1<<31), R0
	// EOR	$(1<<31), R1
	// MOVW	R0, 16(R8)
	// MOVW	R1, 20(R8)

	// MOVW	24(R8), R0
	// MOVW	28(R8), R1
	// EOR	$(1<<31), R0
	// MOVW	R1, 24(R8)
	// MOVW	R0, 28(R8)

	// ADD	$32, R8

	B	loop
loopEnd:
	MOVW	samples_ptr+4(FP), R0
	MOVW	R0, ret_ptr+16(FP)
	MOVW	samples_len+8(FP), R0
	MOVW	R0, ret_len+20(FP)
	MOVW	samples_cap+12(FP), R0
	MOVW	R0, ret_cap+24(FP)
	MOVW	$0, R0
	MOVW	R0, err+28(FP)
	MOVW	R0, err+32(FP)
	RET
