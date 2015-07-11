#include "textflag.h"

TEXT ·FastAtan2(SB),NOSPLIT,$0
	JMP ·fastAtan2(SB)

TEXT ·FastAtan2_2(SB),NOSPLIT,$0
	JMP ·fastAtan2_2(SB)

TEXT ·VAbsC64(SB),NOSPLIT,$0
	JMP ·vAbsC64(SB)

TEXT ·VMaxF32(SB),NOSPLIT,$0
	JMP ·vMaxF32(SB)

TEXT ·VMulC64xF32(SB),NOSPLIT,$0
	JMP ·vMulC64xF32(SB)

TEXT ·VScaleF32(SB),NOSPLIT,$0
	MOVQ	input+0(FP), SI
	MOVQ	input_len+8(FP), AX
	MOVQ	output+24(FP), DI
	MOVQ	output_len+32(FP), CX
	MOVSS	scale+48(FP), X8
	PSHUFD	$0, X8, X8

	CMPQ	AX, CX
	JGE	scalef32_min_len
	MOVQ	AX, CX
scalef32_min_len:

	MOVQ	CX, DX
	ANDQ	$(~31), CX

	MOVQ	$0, AX
	CMPQ	AX, CX
	JGE	scalef32_stepper

scalef32_loop:
	MOVUPS	(SI), X0
	MOVUPS	16(SI), X1
	MOVUPS	32(SI), X2
	MOVUPS	48(SI), X3
	LEAQ	(DI)(AX*4), BX
	MULPS	X8, X0
	MULPS	X8, X1
	MULPS	X8, X2
	MULPS	X8, X3
	MOVUPS	X0, (BX)
	MOVUPS	X1, 16(BX)
	MOVUPS	X2, 32(BX)
	MOVUPS	X3, 48(BX)
	ADDQ	$64, SI
	ADDQ	$16, AX
	CMPQ	AX, CX
	JLT	scalef32_loop

scalef32_stepper:
	CMPQ	AX, DX
	JGE	scalef32_done

scalef32_step:
	MOVSS	(SI), X0
	LEAQ	(DI)(AX*4), BX
	MULSS	X8, X0
	MOVSS	X0, (BX)
	ADDQ	$4, SI
	INCQ	AX
	CMPQ	AX, DX
	JLT	scalef32_step

scalef32_done:
	RET
