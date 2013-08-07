
TEXT ·Ui8toi16b(SB),7,$0
	MOVQ	input+0(FP), R11
	MOVQ	input_len+8(FP), R9
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), BX
	// Choose the shortest length
	SARQ	$1, BX
	CMPQ	BX, R9
	JGE	L1
	MOVQ	BX, R9
L1:
	// Calculate end of input
	ADDQ	R11, R9
loop:
	CMPQ	R11, R9
	JGE	done

	MOVBQZX	(R11), CX
	INCQ	R11

	SUBQ	$128, CX

	MOVB	CX, (DI)
	INCQ	DI
	MOVB	CX, (DI)
	INCQ	DI

	JMP	loop
done:
	RET
