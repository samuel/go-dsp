#include "textflag.h"

TEXT ·Ui8toi16(SB),NOSPLIT,$0-48
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
	MOVQ	$0x8080808080808080, AX
	MOVQ	AX, X8
	PUNPCKLBW	X8, X8
	MOVQ	BX, AX
	SHRQ	$5, AX
	JZ	ui8toi16_tail
ui8toi16_aligned_loop:
	MOVOU	(SI), X0
	MOVOU	16(SI), X1
	PSUBB	X8, X0
	PSUBB	X8, X1
	MOVO	X0, X9
	PUNPCKLBW	X0, X0
	PUNPCKHBW	X9, X9
	MOVO	X0, (DI)
	MOVO	X9, 16(DI)
	MOVO	X1, X9
	PUNPCKLBW	X1, X1
	PUNPCKHBW	X9, X9
	MOVO	X1, 32(DI)
	MOVO	X9, 48(DI)
	ADDQ	$32, SI
	ADDQ	$64, DI
	SUBQ	$32, BX
	DECQ	AX
	JNZ	ui8toi16_aligned_loop

	// TODO: work smaller sizes of blocks

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


TEXT ·Ui8toi16b(SB),NOSPLIT,$0-48
	MOVQ	output_len+32(FP), CX
	SHRQ	$1, CX
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

	MOVQ	$0x8080808080808080, AX
	MOVQ	AX, X8
	PUNPCKLBW	X8, X8
	MOVQ	BX, AX
	SHRQ	$5, AX
	JZ	ui8toi16b_tail
ui8toi16b_aligned_loop:
	MOVOU	(SI), X0
	PSUBB	X8, X0
	MOVOU	16(SI), X1
	PSUBB	X8, X1
	MOVO	X0, X9
	PUNPCKLBW	X0, X0
	PUNPCKHBW	X9, X9
	MOVOU	X0, (DI)
	MOVOU	X9, 16(DI)
	MOVO	X1, X9
	PUNPCKLBW	X1, X1
	PUNPCKHBW	X9, X9
	MOVOU	X1, 32(DI)
	MOVOU	X9, 48(DI)
	ADDQ	$32, SI
	ADDQ	$64, DI
	SUBQ	$32, BX
	DECQ	AX
	JNZ	ui8toi16b_aligned_loop

	// TODO: work increasingly smaller blocks

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



TEXT ·Ui8tof32(SB),NOSPLIT,$0-48
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
	ANDQ	$(~15), DX
	CMPQ	AX, DX
	JGE	ui8tof32_stepper

	MOVQ	$0, BP
	MOVQ	BP, X9
	MOVL	$0x80808080, BX
	MOVL	BX, X8
	PUNPCKLBW	X9, X8
	PUNPCKLWL	X9, X8
ui8tof32_loop:
	MOVOU	(SI), X0	// Load 16 unsigned 8-bit values
	MOVO	X0, X1

	// TODO: Optionally if SSE4.1 is available can use PMOVSXBD or just PMOVZXBD

	// Lowest 4 values (bytes 0-3)
	PUNPCKLBW	X9, X0	// Zero extend low 8-bit to 16-bit
	MOVO	X0, X10
	PUNPCKLWL	X9, X0	// Zero extend low 16-bit to 32-bit
	PSUBL	X8, X0		// Subtract 128 to convert to signed values
	CVTPL2PS	X0, X0	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X0, (DI)

	// Next 4 values (bytes 4-7)
	PUNPCKHWL	X9, X10	// Zero extend high 16-bit to 32-bit
	PSUBL	X8, X10		// Subtract 128 to conver to signed values
	CVTPL2PS	X10, X10	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X10, 16(DI)

	// Next 4 values (bytes 8-11)
	PUNPCKHBW	X9, X1	// Zero extend high 8-bit to 16-bit
	MOVO	X1, X10
	PUNPCKLWL	X9, X1	// Zero extend low 16-bit to 32-bit
	PSUBL	X8, X1		// Subtract 128 to convert to signed values
	CVTPL2PS	X1, X1	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X1, 32(DI)

	// Next 4 values (bytes 12-15)
	PUNPCKHWL	X9, X10	// Zero extend high 16-bit to 32-bit
	PSUBL	X8, X10		// Subtract 128 to convert to signed values
	CVTPL2PS	X10, X10	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X10, 48(DI)

	ADDQ	$16, SI
	ADDQ	$64, DI
	ADDQ	$16, AX
	CMPQ	AX, DX
	JLT	ui8tof32_loop

	// TODO: work increasingly smaller blocks

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


TEXT ·I8tof32(SB),NOSPLIT,$0-48
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), AX
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), CX

	CMPQ	AX, CX
	JGE	i8tof32_min_len
	MOVQ	AX, CX
i8tof32_min_len:

	MOVQ	$0, AX

	// Too short to optimize
	MOVQ 	$32, BX
	CMPQ	BX, CX
	JGE	i8tof32_stepper

	// Align output to 16-byte boundary
	MOVQ	DI, BP
	ANDQ	$15, BP
	SHRQ	$2, BP
	JZ	i8tof32_aligned
	MOVQ	$16, DX
	SUBQ	BP, DX
i8tof32_align:
	MOVBQSX	(SI), BX
	INCQ	SI
	CVTSQ2SS	BX, X0
	MOVSS	X0, (DI)
	ADDQ	$4, DI
	INCQ	AX
	DECQ	DX
	JNZ	i8tof32_align
i8tof32_aligned:

	MOVQ	CX, DX
	ANDQ	$(~15), DX
	CMPQ	AX, DX
	JGE	i8tof32_stepper

	MOVQ	$0, BP
	MOVQ	BP, X9
	MOVL	$0x80808080, BX
	MOVL	BX, X8
	MOVO	X8, X11
	PUNPCKLBW	X9, X8
	PUNPCKLWL	X9, X8
	PUNPCKLBW	X11, X11
	PUNPCKLWL	X11, X11
i8tof32_loop:
	MOVOU	(SI), X0	// Load 16 unsigned 8-bit values
	PADDB	X11, X0		// Make values unsigned
	MOVO	X0, X1

	// TODO: Optionally if SSE4.1 is available can use PMOVSXBD or just PMOVZXBD

	// Lowest 4 values (bytes 0-3)
	PUNPCKLBW	X9, X0	// Zero extend low 8-bit to 16-bit
	MOVO	X0, X10
	PUNPCKLWL	X9, X0	// Zero extend low 16-bit to 32-bit
	PSUBL	X8, X0		// Subtract 128 to convert to signed values
	CVTPL2PS	X0, X0	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X0, (DI)

	// Next 4 values (bytes 4-7)
	PUNPCKHWL	X9, X10	// Zero extend high 16-bit to 32-bit
	PSUBL	X8, X10		// Subtract 128 to conver to signed values
	CVTPL2PS	X10, X10	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X10, 16(DI)

	// Next 4 values (bytes 8-11)
	PUNPCKHBW	X9, X1	// Zero extend high 8-bit to 16-bit
	MOVO	X1, X10
	PUNPCKLWL	X9, X1	// Zero extend low 16-bit to 32-bit
	PSUBL	X8, X1		// Subtract 128 to convert to signed values
	CVTPL2PS	X1, X1	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X1, 32(DI)

	// Next 4 values (bytes 12-15)
	PUNPCKHWL	X9, X10	// Zero extend high 16-bit to 32-bit
	PSUBL	X8, X10		// Subtract 128 to convert to signed values
	CVTPL2PS	X10, X10	// Convert 32-bit signed integers to 32-bit float
	MOVAPS	X10, 48(DI)

	ADDQ	$16, SI
	ADDQ	$64, DI
	ADDQ	$16, AX
	CMPQ	AX, DX
	JLT	i8tof32_loop

	// TODO: work increasingly smaller blocks

i8tof32_stepper:
	CMPQ	AX, CX
	JGE	i8tof32_done
i8tof32_step:
	MOVBQSX	(SI), BX
	INCQ	SI
	CVTSQ2SS	BX, X0
	MOVSS	X0, (DI)
	ADDQ	$4, DI
	INCQ	AX
	CMPQ	AX, CX
	JLT	i8tof32_step
i8tof32_done:
	RET


TEXT ·Ui8toc64(SB),NOSPLIT,$0-48
	MOVQ	input_len+8(FP), AX
	ANDQ	$-2, AX
	MOVQ	AX, input_len+8(FP)
	MOVQ	output_len+32(FP), CX
	SHLQ	$1, CX
	MOVQ	CX, output_len+32(FP)
	JMP	·Ui8tof32(SB)


TEXT ·F32toi16(SB),NOSPLIT,$0-52
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), AX
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), CX
	MOVSS	scale+48(FP), X8
	PSHUFD	$0, X8, X8

	CMPQ	AX, CX
	JGE	f32toi16_min_len
	MOVQ	AX, CX
f32toi16_min_len:

	MOVQ	CX, DX
	ANDQ	$(~31), CX

	MOVQ	$0, AX
	CMPQ	AX, CX
	JGE	f32toi16_stepper

f32toi16_loop:
	MOVUPS	(SI), X0
	MOVUPS	16(SI), X1
	MOVUPS	32(SI), X2
	MOVUPS	48(SI), X3
	LEAQ	(DI)(AX*2), BX
	MULPS	X8, X0
	MULPS	X8, X1
	MULPS	X8, X2
	MULPS	X8, X3
	CVTTPS2PL	X0, X0
	CVTTPS2PL	X1, X1
	CVTTPS2PL	X2, X2
	CVTTPS2PL	X3, X3
	PACKSSLW	X1, X0
	PACKSSLW	X3, X2
	MOVOU	X0, (BX)
	MOVOU	X2, 16(BX)
	ADDQ	$64, SI
	ADDQ	$16, AX
	CMPQ	AX, CX
	JLT	f32toi16_loop

f32toi16_stepper:
	CMPQ	AX, DX
	JGE	f32toi16_done

f32toi16_step:
	MOVSS	(SI), X0
	LEAQ	(DI)(AX*2), BX
	MULSS	X8, X0
	CVTTSS2SL	X0, BP
	MOVW	BP, (BX)
	ADDQ	$4, SI
	INCQ	AX
	CMPQ	AX, DX
	JLT	f32toi16_step

f32toi16_done:
	RET



TEXT ·F32toi16ble(SB),NOSPLIT,$0-52
	MOVQ	output_len+32(FP), AX
	SHRQ	$1, AX
	MOVQ	AX, output_len+32(FP)
	JMP	·F32toi16(SB)
