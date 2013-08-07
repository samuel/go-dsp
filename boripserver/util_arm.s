
TEXT ·Ui8toi16(SB),7,$0
	MOVW	input+0(FP),R1
	MOVW   	input_len+4(FP),R2
	MOVW	output+12(FP), R3
	MOVW	output_len+12(FP), R4
	// Choose the shortest length
	MOVW	R4>>1, R4
	CMP	R2, R4
	MOVW.LT	R4, R2
	// If no input then skip loop
	CMP	$0, R2
	BEQ	done
	// Calculate end of input
	ADD	R1, R2
loop:
	MOVBU	0(R1),R0
	ADD	$1,R1

	SUB	$128,R0

	MOVBU  	R0,0(R3)
	MOVBU  	R0,1(R3)
	ADD	$2,R3

	CMP	R2,R1
	BLT	loop
done:
	RET
