
TEXT ·Ui8toi16b(SB),7,$0
	MOVQ	input+0(FP), R11
	MOVQ	input_len+8(FP), R9
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), BX
	// Choose the shortest length
	SARQ	$1, BX
	CMPQ	BX, R9
	JGE	ui8toi16b_L1
	MOVQ	BX, R9
ui8toi16b_L1:
	// Calculate end of input
	ADDQ	R11, R9
ui8toi16b_loop:
	CMPQ	R11, R9
	JGE	ui8toi16b_done

	MOVBQZX	(R11), CX
	INCQ	R11

	SUBQ	$128, CX

	MOVB	CX, (DI)
	INCQ	DI
	MOVB	CX, (DI)
	INCQ	DI

	JMP	ui8toi16b_loop
ui8toi16b_done:
	RET

TEXT ·Ui8toc64(SB),7,$0
	JMP ·ui8toc64(SB)

TEXT ·F32toi16b(SB),7,$0
	JMP ·f32toi16b(SB)
