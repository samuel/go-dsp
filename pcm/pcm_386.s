//go:build !386.softfloat

#include "textflag.h"

// SSE2 kernels for 386, the same steps as the generated amd64 SSE2 leaves:
// decoding widens to int32, converts and multiplies by 2^-(bits-1); encoding
// multiplies by 32768, masks NaN to zero, clips to the int16 rails and
// converts with CVTPS2PL, which rounds to nearest even under the default
// MXCSR that Go keeps.

DATA bias8<>+0(SB)/4, $0x80808080
GLOBL bias8<>(SB), RODATA|NOPTR, $4
DATA inv128F32<>+0(SB)/4, $0x3c000000 // 1/128
GLOBL inv128F32<>(SB), RODATA|NOPTR, $4
DATA inv32768F32<>+0(SB)/4, $0x38000000 // 1/32768
GLOBL inv32768F32<>(SB), RODATA|NOPTR, $4
DATA mul32768F32<>+0(SB)/4, $0x47000000 // 32768
GLOBL mul32768F32<>(SB), RODATA|NOPTR, $4
DATA i16MinF32<>+0(SB)/4, $0xc7000000 // -32768
GLOBL i16MinF32<>(SB), RODATA|NOPTR, $4
DATA i16MaxF32<>+0(SB)/4, $0x46fffe00 // 32767
GLOBL i16MaxF32<>(SB), RODATA|NOPTR, $4

// BYTES_TO_F32 converts the sixteen signed bytes in X0 to float32 at DI,
// scaled by X7. Each byte is duplicated up to the top of a 32-bit lane and
// shifted back down arithmetically.
#define BYTES_TO_F32 \
	MOVO      X0, X1     \
	PUNPCKLBW X1, X1     \
	PUNPCKHBW X0, X0     \
	MOVO      X1, X2     \
	PUNPCKLWL X1, X1     \
	PUNPCKHWL X2, X2     \
	MOVO      X0, X3     \
	PUNPCKLWL X0, X0     \
	PUNPCKHWL X3, X3     \
	PSRAL     $24, X1    \
	PSRAL     $24, X2    \
	PSRAL     $24, X0    \
	PSRAL     $24, X3    \
	CVTPL2PS  X1, X1     \
	CVTPL2PS  X2, X2     \
	CVTPL2PS  X0, X0     \
	CVTPL2PS  X3, X3     \
	MULPS     X7, X1     \
	MULPS     X7, X2     \
	MULPS     X7, X0     \
	MULPS     X7, X3     \
	MOVUPS    X1, (DI)   \
	MOVUPS    X2, 16(DI) \
	MOVUPS    X0, 32(DI) \
	MOVUPS    X3, 48(DI)

// func u8ToF32(dst []float32, src []byte)
TEXT ·u8ToF32(SB), NOSPLIT, $0-24
	MOVL dst_base+0(FP), DI
	MOVL dst_len+4(FP), CX
	MOVL src_base+12(FP), SI
	MOVL src_len+16(FP), AX
	CMPL AX, CX
	JGE  u8_min
	MOVL AX, CX

u8_min:
	MOVSS  inv128F32<>(SB), X7
	SHUFPS $0, X7, X7
	MOVSS  bias8<>(SB), X6
	PSHUFL $0, X6, X6

u8_loop:
	CMPL CX, $16
	JLT  u8_tail
	MOVOU (SI), X0
	PXOR  X6, X0
	BYTES_TO_F32
	ADDL $16, SI
	ADDL $64, DI
	SUBL $16, CX
	JMP  u8_loop

u8_tail:
	TESTL CX, CX
	JZ    u8_done

u8_step:
	MOVBLZX  (SI), AX
	SUBL     $128, AX
	CVTSL2SS AX, X0
	MULSS    X7, X0
	MOVSS    X0, (DI)
	INCL     SI
	ADDL     $4, DI
	DECL     CX
	JNZ      u8_step

u8_done:
	RET

// func i8ToF32(dst []float32, src []byte)
TEXT ·i8ToF32(SB), NOSPLIT, $0-24
	MOVL dst_base+0(FP), DI
	MOVL dst_len+4(FP), CX
	MOVL src_base+12(FP), SI
	MOVL src_len+16(FP), AX
	CMPL AX, CX
	JGE  i8_min
	MOVL AX, CX

i8_min:
	MOVSS  inv128F32<>(SB), X7
	SHUFPS $0, X7, X7

i8_loop:
	CMPL CX, $16
	JLT  i8_tail
	MOVOU (SI), X0
	BYTES_TO_F32
	ADDL $16, SI
	ADDL $64, DI
	SUBL $16, CX
	JMP  i8_loop

i8_tail:
	TESTL CX, CX
	JZ    i8_done

i8_step:
	MOVBLSX  (SI), AX
	CVTSL2SS AX, X0
	MULSS    X7, X0
	MOVSS    X0, (DI)
	INCL     SI
	ADDL     $4, DI
	DECL     CX
	JNZ      i8_step

i8_done:
	RET

// func i16LEToF32(dst []float32, src []byte)
TEXT ·i16LEToF32(SB), NOSPLIT, $0-24
	MOVL dst_base+0(FP), DI
	MOVL dst_len+4(FP), CX
	MOVL src_base+12(FP), SI
	MOVL src_len+16(FP), AX
	SHRL $1, AX
	CMPL AX, CX
	JGE  i16_min
	MOVL AX, CX

i16_min:
	MOVSS  inv32768F32<>(SB), X7
	SHUFPS $0, X7, X7

i16_loop:
	CMPL CX, $8
	JLT  i16_tail
	MOVOU     (SI), X0
	MOVO      X0, X1
	PUNPCKLWL X1, X1
	PUNPCKHWL X0, X0
	PSRAL     $16, X1
	PSRAL     $16, X0
	CVTPL2PS  X1, X1
	CVTPL2PS  X0, X0
	MULPS     X7, X1
	MULPS     X7, X0
	MOVUPS    X1, (DI)
	MOVUPS    X0, 16(DI)
	ADDL      $16, SI
	ADDL      $32, DI
	SUBL      $8, CX
	JMP       i16_loop

i16_tail:
	TESTL CX, CX
	JZ    i16_done

i16_step:
	MOVWLSX  (SI), AX
	CVTSL2SS AX, X0
	MULSS    X7, X0
	MOVSS    X0, (DI)
	ADDL     $2, SI
	ADDL     $4, DI
	DECL     CX
	JNZ      i16_step

i16_done:
	RET

// func f32ToI16LE(dst []byte, src []float32)
TEXT ·f32ToI16LE(SB), NOSPLIT, $0-24
	MOVL dst_base+0(FP), DI
	MOVL dst_len+4(FP), CX
	MOVL src_base+12(FP), SI
	MOVL src_len+16(FP), AX
	SHRL $1, CX
	CMPL AX, CX
	JGE  enc_min
	MOVL AX, CX

enc_min:
	MOVSS  mul32768F32<>(SB), X5
	SHUFPS $0, X5, X5
	MOVSS  i16MinF32<>(SB), X6
	SHUFPS $0, X6, X6
	MOVSS  i16MaxF32<>(SB), X7
	SHUFPS $0, X7, X7

enc_loop:
	CMPL CX, $8
	JLT  enc_tail
	MOVUPS   (SI), X0
	MOVUPS   16(SI), X1
	MULPS    X5, X0
	MULPS    X5, X1
	MOVAPS   X0, X2
	MOVAPS   X1, X3
	CMPPS    X2, X2, $0
	CMPPS    X3, X3, $0
	ANDPS    X2, X0
	ANDPS    X3, X1
	MAXPS    X6, X0
	MAXPS    X6, X1
	MINPS    X7, X0
	MINPS    X7, X1
	CVTPS2PL X0, X0
	CVTPS2PL X1, X1
	PACKSSLW X1, X0
	MOVOU    X0, (DI)
	ADDL     $32, SI
	ADDL     $16, DI
	SUBL     $8, CX
	JMP      enc_loop

enc_tail:
	TESTL CX, CX
	JZ    enc_done

enc_step:
	MOVSS    (SI), X0
	MULSS    X5, X0
	MOVAPS   X0, X2
	CMPSS    X2, X2, $0
	ANDPS    X2, X0
	MAXSS    X6, X0
	MINSS    X7, X0
	CVTSS2SL X0, AX
	MOVW     AX, (DI)
	ADDL     $4, SI
	ADDL     $2, DI
	DECL     CX
	JNZ      enc_step

enc_done:
	RET
