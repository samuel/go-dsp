#include "go_asm.h"
#include "textflag.h"

TEXT ·FastAtan2(SB), NOSPLIT, $0
	JMP ·fastAtan2(SB)

TEXT ·FastAtan2_2(SB), NOSPLIT, $0
	JMP ·fastAtan2_2(SB)

TEXT ·VAbsC64(SB), NOSPLIT, $0
	JMP ·vAbsC64(SB)

TEXT ·VMaxF32(SB), NOSPLIT, $0-28
	MOVQ input+0(FP), SI
	MOVQ input_len+8(FP), CX

	MOVL $0xff800000, AX // -InF
	MOVL AX, X0

	MOVQ $0, DX

	//CMPB	internal∕cpu·X86+const_offsetX86HasAVX2(SB), $1
	CMPB	·useAVX2(SB), $1
	JE 		vmaxf32_avx2
	//CMPB 	internal∕cpu·X86+const_offsetX86HasSSE2(SB), $1
	CMPB	·useSSE2(SB), $1
	JE 		vmaxf32_sse2
	JMP		vmaxf32_scalar

vmaxf32_avx2:
	MOVQ CX, BX
	ANDQ $-32, BX
	CMPQ DX, BX
	JGE  vmaxf32_scalar

	VBROADCASTSS X0, Y0
	VMOVUPS	Y0, Y1
	VMOVUPS	Y0, Y2
	VMOVUPS	Y0, Y3

vmaxf32_avx2_loop:
	VMOVUPS (SI), Y4
	VMOVUPS 32(SI), Y5
	VMOVUPS 64(SI), Y6
	VMOVUPS 96(SI), Y7
	VMAXPS  Y4, Y0, Y0
	VMAXPS  Y5, Y1, Y1
	VMAXPS  Y6, Y2, Y2
	VMAXPS  Y7, Y3, Y3
	ADDQ    $128, SI
	ADDQ    $32, DX
	CMPQ    DX, BX
	JLT     vmaxf32_avx2_loop

	VMAXPS	Y1, Y0, Y0
	VMAXPS	Y2, Y0, Y0
	VMAXPS	Y3, Y0, Y0
	VEXTRACTF128 $1, Y0, X1
	MAXPS   X1, X0
	MOVHLPS X0, X1
	MAXPS	X1, X0
	PSHUFD  $0x55, X0, X1
	MAXPS   X1, X0
	JMP		vmaxf32_scalar

vmaxf32_sse2:
	MOVQ CX, BX
	ANDQ $-16, BX
	CMPQ DX, BX
	JGE  vmaxf32_scalar

	PSHUFD	$0, X0, X0
	MOVUPS	X0, X1
	MOVUPS	X0, X2
	MOVUPS	X0, X3

vmaxf32_sse_loop:
	MOVUPS (SI), X4
	MOVUPS 16(SI), X5
	MOVUPS 32(SI), X6
	MOVUPS 48(SI), X7
	MAXPS  X4, X0
	MAXPS  X5, X1
	MAXPS  X6, X2
	MAXPS  X7, X3
	ADDQ   $64, SI
	ADDQ   $16, DX
	CMPQ   DX, BX
	JLT    vmaxf32_sse_loop

	MAXPS	X1, X0
	MAXPS	X2, X0
	MAXPS	X3, X0
	MOVHLPS X0, X1
	MAXPS   X1, X0
	PSHUFD  $0x55, X0, X1
	MAXPS   X1, X0

vmaxf32_scalar:
	CMPQ DX, CX
	JGE  vmaxf32_done

vmaxf32_scalar_loop:
	MOVSS   (SI), X1
	UCOMISS X0, X1
	JLS     vmaxf32_not_max
	MOVO    X1, X0

vmaxf32_not_max:
	ADDQ $4, SI
	INCQ DX
	CMPQ DX, CX
	JLT  vmaxf32_scalar_loop

vmaxf32_done:
	MOVSS X0, ret+24(FP)
	RET

TEXT ·VMinF32(SB), NOSPLIT, $0-28
	MOVQ input+0(FP), SI
	MOVQ input_len+8(FP), CX

	MOVL $0x7f800000, AX // InF
	MOVL AX, X0

	MOVQ $0, DX

	//CMPB	·x86+const_offsetX86HasAVX2(SB), $1
	CMPB	·useAVX2(SB), $1
	JE 		vminf32_avx2
	//CMPB	·x86+const_offsetX86HasSSE2(SB), $1
	CMPB	·useSSE2(SB), $1
	JE 		vminf32_sse2
	JMP		vminf32_scalar

vminf32_avx2:
	MOVQ CX, BX
	ANDQ $-32, BX
	CMPQ DX, BX
	JGE  vminf32_scalar

	VBROADCASTSS X0, Y0
	VMOVUPS	Y0, Y1
	VMOVUPS	Y0, Y2
	VMOVUPS	Y0, Y3

vminf32_avx2_loop:
	VMOVUPS (SI), Y4
	VMOVUPS 32(SI), Y5
	VMOVUPS 64(SI), Y6
	VMOVUPS 96(SI), Y7
	VMINPS  Y4, Y0, Y0
	VMINPS  Y5, Y1, Y1
	VMINPS  Y6, Y2, Y2
	VMINPS  Y7, Y3, Y3
	ADDQ    $128, SI
	ADDQ    $32, DX
	CMPQ    DX, BX
	JLT     vminf32_avx2_loop

	VMINPS	Y1, Y0, Y0
	VMINPS	Y2, Y0, Y0
	VMINPS	Y3, Y0, Y0
	VEXTRACTF128 $1, Y0, X1
	MINPS   X1, X0
	MOVHLPS X0, X1
	MINPS	X1, X0
	PSHUFD  $0x55, X0, X1
	MINPS   X1, X0
	JMP		vminf32_scalar

vminf32_sse2:
	MOVQ CX, BX
	ANDQ $-16, BX
	CMPQ DX, BX
	JGE  vminf32_scalar

	PSHUFD	$0, X0, X0
	MOVUPS	X0, X1
	MOVUPS	X0, X2
	MOVUPS	X0, X3

vminf32_sse_loop:
	MOVUPS (SI), X4
	MOVUPS 16(SI), X5
	MOVUPS 32(SI), X6
	MOVUPS 48(SI), X7
	MINPS  X4, X0
	MINPS  X5, X1
	MINPS  X6, X2
	MINPS  X7, X3
	ADDQ   $64, SI
	ADDQ   $16, DX
	CMPQ   DX, BX
	JLT    vminf32_sse_loop

	MINPS	X1, X0
	MINPS	X2, X0
	MINPS	X3, X0
	MOVHLPS X0, X1
	MINPS   X1, X0
	PSHUFD  $0x55, X0, X1
	MINPS   X1, X0

vminf32_scalar:
	CMPQ DX, CX
	JGE  vminf32_done

vminf32_scalar_loop:
	MOVSS   (SI), X1
	UCOMISS X1, X0
	JLS     vminf32_not_min
	MOVO    X1, X0

vminf32_not_min:
	ADDQ $4, SI
	INCQ DX
	CMPQ DX, CX
	JLT  vminf32_scalar_loop

vminf32_done:
	MOVSS X0, ret+24(FP)
	RET

TEXT ·VMulC64xF32(SB), NOSPLIT, $0
	JMP ·vMulC64xF32(SB)

TEXT ·VScaleF32(SB), NOSPLIT, $0
	MOVQ   input+0(FP), SI
	MOVQ   input_len+8(FP), AX
	MOVQ   output+24(FP), DI
	MOVQ   output_len+32(FP), CX
	MOVSS  scale+48(FP), X8
	PSHUFD $0, X8, X8

	CMPQ AX, CX
	JGE  vscalef32_min_len
	MOVQ AX, CX
vscalef32_min_len:
	MOVQ CX, DX

	MOVQ $0, AX

	//CMPB	·x86+const_offsetX86HasAVX2(SB), $1
	CMPB	·useAVX2(SB), $1
	JE 		vscalef32_avx2
	//CMPB	·x86+const_offsetX86HasSSE2(SB), $1
	CMPB	·useSSE2(SB), $1
	JE 		vscalef32_sse2
	JMP		vscalef32_scalar

vscalef32_avx2:
	MOVQ CX, DX
	ANDQ $(~63), CX
	CMPQ AX, CX
	JGE  vscalef32_scalar

	VBROADCASTSS X8, Y8

vscalef32_avx2_loop:
	VMOVUPS (SI), Y0
	VMOVUPS 32(SI), Y1
	VMOVUPS 64(SI), Y2
	VMOVUPS 96(SI), Y3
	VMULPS  Y8, Y0, Y0
	VMULPS  Y8, Y1, Y1
	VMULPS  Y8, Y2, Y2
	VMULPS  Y8, Y3, Y3
	VMOVUPS Y0, (DI)
	VMOVUPS Y1, 32(DI)
	VMOVUPS Y2, 64(DI)
	VMOVUPS Y3, 96(DI)
	ADDQ   $32, AX
	ADDQ   $128, SI
	ADDQ   $128, DI
	CMPQ   AX, CX
	JLT    vscalef32_avx2_loop

	JMP    vscalef32_scalar

vscalef32_sse2:
	MOVQ CX, DX
	ANDQ $(~31), CX
	CMPQ AX, CX
	JGE  vscalef32_scalar

vscalef32_sse2_loop:
	MOVUPS (SI), X0
	MOVUPS 16(SI), X1
	MOVUPS 32(SI), X2
	MOVUPS 48(SI), X3
	MULPS  X8, X0
	MULPS  X8, X1
	MULPS  X8, X2
	MULPS  X8, X3
	MOVUPS X0, (DI)
	MOVUPS X1, 16(DI)
	MOVUPS X2, 32(DI)
	MOVUPS X3, 48(DI)
	ADDQ   $16, AX
	ADDQ   $64, SI
	ADDQ   $64, DI
	CMPQ   AX, CX
	JLT    vscalef32_sse2_loop

vscalef32_scalar:
	CMPQ AX, DX
	JGE  vscalef32_done

vscalef32_scalar_loop:
	MOVSS (SI), X0
	MULSS X8, X0
	MOVSS X0, (DI)
	INCQ  AX
	ADDQ  $4, SI
	ADDQ  $4, DI
	CMPQ  AX, CX
	JLT   vscalef32_scalar_loop

vscalef32_done:
	RET
