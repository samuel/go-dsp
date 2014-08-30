
TEXT ·Ui8toi16(SB),7,$0
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), BX
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), CX

	CMPQ	CX, BX
	JGE	ui8toi16_min_length
	MOVQ	CX, BX
ui8toi16_min_length:
	// BX = length

	MOVQ	$32, AX
	CMPQ	AX, BX
	JG	ui8toi16_tail

	// Single step to align output to 16-bytes
	MOVQ	DI, CX
	ANDQ	$15, CX
	JZ	ui8toi16_aligned
	MOVQ	$16, AX
	SUBQ	CX, AX
	SHRQ	$1, AX
ui8toi16_head_loop:
	MOVBQZX	(SI), CX
	INCQ	SI
	SUBQ	$128, CX
	MOVB	CX, (DI)
	INCQ	DI
	MOVB	CX, (DI)
	INCQ	DI
	DECQ	BX
	DECQ	AX
	JNZ	ui8toi16_head_loop

ui8toi16_aligned:
	// Work 8 bytes at a time
	MOVQ	$0x8080808080808080, AX
	MOVQ	AX, X1
	MOVQ	BX, AX
	SHRQ	$3, AX
	JZ	ui8toi16_tail
ui8toi16_aligned_loop:
	MOVQ	(SI), X0
	ADDQ	$8, SI
	PSUBB	X1, X0
	PUNPCKLBW	X0, X0
	MOVO	X0, (DI)
	ADDQ	$16, DI
	SUBQ	$8, BX
	DECQ	AX
	JNZ	ui8toi16_aligned_loop

ui8toi16_tail:
	// Single step anything that is left
	ANDQ	BX, BX
	JZ	ui8toi16_done
ui8toi16_tail_loop:
 	MOVBQZX	(SI), CX
 	INCQ	SI
 	SUBQ	$128, CX
 	MOVB	CX, (DI)
 	INCQ	DI
 	MOVB	CX, (DI)
 	INCQ	DI
	DECQ	BX
	JNZ	ui8toi16_tail_loop

ui8toi16_done:
	RET



TEXT ·Ui8toi16b(SB),7,$0
	MOVQ	output_len+32(FP), CX
	SHRQ	$1, CX	// output_len /= 2
	MOVQ	output+24(FP), DI
	MOVQ	DI, AX
	ANDQ	$1, AX
	JNZ	ui8toi16b_unaligned

	// Aligned version can just use Ui8toi16 but with adjusted output length.
	MOVQ	CX, output_len+32(FP)
	JMP	·Ui8toi16(SB)

ui8toi16b_unaligned:
	// Output is on an odd address which means it cannot be aligned
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), BX

	// Choose the shortest length
	CMPQ	CX, BX
	JGE	ui8toi16b_min_length
	MOVQ	CX, BX
ui8toi16b_min_length:
	// BX = length

	// Work 8 bytes at a time
	MOVQ	$0x8080808080808080, AX
	MOVQ	AX, X1
	MOVQ	BX, AX
	SHRQ	$3, AX
	JZ	ui8toi16b_tail
ui8toi16b_loop:
	MOVQ	(SI), X0
	ADDQ	$8, SI
	PSUBB	X1, X0
	PUNPCKLBW	X0, X0
	MOVOU	X0, (DI)
	ADDQ	$16, DI
	SUBQ	$8, BX
	DECQ	AX
	JNZ	ui8toi16b_loop

ui8toi16b_tail:
	// Single step anything that is left
	ANDQ	BX, BX
	JZ	ui8toi16b_done
ui8toi16l_tail_loop:
 	MOVBQZX	(SI), CX
 	INCQ	SI
 	SUBQ	$128, CX
 	MOVB	CX, (DI)
 	INCQ	DI
 	MOVB	CX, (DI)
 	INCQ	DI
	DECQ	BX
	JNZ	ui8toi16l_tail_loop

ui8toi16b_done:
	RET



TEXT ·Ui8tof32(SB),7,$0
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), AX
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), CX

	CMPQ	AX, CX
	JGE	ui8tof32_min_len
	MOVQ	AX, CX
ui8tof32_min_len:

	MOVQ	$0, AX

	// Too short to optimize
	MOVQ 	$32, BX
	CMPQ	BX, CX
	JGE	ui8tof32_stepper

	// Align output to 16-byte boundary
	MOVQ	DI, BP
	ANDQ	$15, BP
	SHRQ	$2, BP
	JZ	ui8tof32_aligned
	MOVQ	$16, DX
	SUBQ	BP, DX
ui8tof32_align:
	MOVBQZX	(SI), BX
	INCQ	SI
	SUBQ	$128, BX
	CVTSQ2SS	BX, X0
	MOVSS	X0, (DI)
	ADDQ	$4, DI
	INCQ	AX
	DECQ	DX
	JNZ	ui8tof32_align
ui8tof32_aligned:

	MOVQ	CX, DX
	ANDQ	$0xfffffffffffffffc, DX
	CMPQ	AX, DX
	JGE	ui8tof32_stepper

	// Convert 4 values at a time
	MOVQ	AX, X2
	MOVL	$0x80808080, BX
	MOVL	BX, X1
	PUNPCKLBW	X2, X1
	PUNPCKLWL	X2, X1
ui8tof32_loop:
	MOVL	(SI), X0
	ADDQ	$4, SI
	PUNPCKLBW	X2, X0
	PUNPCKLWL	X2, X0
	PSUBL	X1, X0
	CVTPL2PS	X0, X0
	MOVO	X0, (DI)
	ADDQ	$16, DI
	ADDQ	$4, AX
	CMPQ	AX, DX
	JLT	ui8tof32_loop

ui8tof32_stepper:
	CMPQ	AX, CX
	JGE	ui8tof32_done
ui8tof32_step:
	MOVBQZX	(SI), BX
	INCQ	SI
	SUBQ	$128, BX
	CVTSQ2SS	BX, X0
	MOVSS	X0, (DI)
	ADDQ	$4, DI
	INCQ	AX
	CMPQ	AX, CX
	JLT	ui8tof32_step
ui8tof32_done:
	RET


TEXT ·Ui8toc64(SB),7,$0
	MOVQ	input_len+8(FP), AX
	ANDQ	$-2, AX
	MOVQ	AX, input_len+8(FP)
	MOVQ	output_len+32(FP), CX
	SHLQ	$1, CX
	MOVQ	CX, output_len+32(FP)
	JMP	·Ui8tof32(SB)


TEXT ·F32toi16(SB),7,$0
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), AX
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), CX
	MOVQ	scale+48(FP), X1
	PSHUFD	$0, X1, X1

	CMPQ	AX, CX
	JGE	f32toi16_min_len
	MOVQ	AX, CX
f32toi16_min_len:

	MOVQ	CX, DX
	ANDQ	$0xfffffffffffffffc, CX

	MOVQ	$0, AX
	CMPQ	AX, CX
	JGE	f32toi16_stepper

f32toi16_loop:
	MOVUPS	(SI), X0
	LEAQ	(DI)(AX*2), BX
	MULPS	X1, X0
	CVTTPS2PL	X0, X2
	PACKSSLW	X2, X2
	MOVQ	X2, (BX)
	ADDQ	$16, SI
	ADDQ	$4, AX
	CMPQ	AX, CX
	JLT	f32toi16_loop

f32toi16_stepper:
	CMPQ	AX, DX
	JGE	f32toi16_done

f32toi16_step:
	MOVSS	(SI), X0
	LEAQ	(DI)(AX*2), BX
	MULSS	X1, X0
	CVTTSS2SL	X0, BP
	MOVW	BP, (BX)
	ADDQ	$4, SI
	INCQ	AX
	CMPQ	AX, DX
	JLT	f32toi16_step

f32toi16_done:
	RET



TEXT ·F32toi16ble(SB),7,$0
	MOVQ	output_len+32(FP), AX
	SHRQ	$1, AX
	MOVQ	AX, output_len+32(FP)
	JMP	·F32toi16(SB)
