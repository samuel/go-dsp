
TEXT ·rotate90FilterAsm(SB),7,$0
	MOVW	samples_len+8(FP), R2
	MOVW   	samples_ptr+4(FP), R5
	AND	$(~3), R2	// round down to nearest multiple of 4
	MOVW	$0, R3
	B	loopStart
loop:
	ADD    	$4, R3
loopStart:
	CMP    	R3, R2
	BLE    	loopEnd

	MOVW	8(R5), R0
	MOVW	12(R5), R1
	EOR	$(1<<31), R1
	MOVW	R1, 0(R5)
	MOVW	R0, 4(R5)

	MOVW	16(R5), R0
	MOVW	20(R5), R1
	EOR	$(1<<31), R0
	EOR	$(1<<31), R1
	MOVW	R0, 0(R5)
	MOVW	R1, 4(R5)

	MOVW	24(R5), R0
	MOVW	28(R5), R1
	EOR	$(1<<31), R0
	MOVW	R1, 0(R5)
	MOVW	R0, 4(R5)

	ADD	$32, R5

	B      	loop
loopEnd:
	MOVW   	samples_ptr+4(FP), R0
	MOVW   	R0, ret_ptr+16(FP)
	MOVW   	samples_len+8(FP), R0
	MOVW   	R0, ret_len+20(FP)
	MOVW   	samples_cap+12(FP), R0
	MOVW   	R0, ret_cap+24(FP)
	MOVW   	$0, R0
	MOVW   	R0, err+28(FP)
	MOVW   	R0, err+32(FP)
	RET
