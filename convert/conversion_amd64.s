#include "go_asm.h"
#include "textflag.h"

TEXT ·u8ToI16Asm(SB), NOSPLIT, $0-48
	MOVQ src+24(FP), SI
	MOVQ src_len+32(FP), BX
	MOVQ dst+0(FP), DI
	MOVQ dst_len+8(FP), CX

	CMPQ CX, BX
	JGE  u8toi16_min_length
	MOVQ CX, BX

u8toi16_min_length:
	// BX = length

	MOVQ $32, AX
	CMPQ AX, BX
	JG   u8toi16_tail

	// Single step to align output to 16-bytes
	MOVQ DI, CX
	ANDQ $15, CX
	JZ   u8toi16_aligned
	MOVQ $16, AX
	SUBQ CX, AX
	SHRQ $1, AX

u8toi16_head_loop:
	MOVBQZX (SI), CX
	INCQ    SI
	SUBQ    $128, CX
	MOVB    CX, (DI)
	INCQ    DI
	MOVB    CX, (DI)
	INCQ    DI
	DECQ    BX
	DECQ    AX
	JNZ     u8toi16_head_loop

u8toi16_aligned:
	MOVQ      $0x8080808080808080, AX
	MOVQ      AX, X8
	PUNPCKLBW X8, X8
	MOVQ      BX, AX
	SHRQ      $5, AX
	JZ        u8toi16_tail

u8toi16_aligned_loop:
	MOVOU     (SI), X0
	MOVOU     16(SI), X1
	PSUBB     X8, X0
	PSUBB     X8, X1
	MOVO      X0, X9
	PUNPCKLBW X0, X0
	PUNPCKHBW X9, X9
	MOVO      X0, (DI)
	MOVO      X9, 16(DI)
	MOVO      X1, X9
	PUNPCKLBW X1, X1
	PUNPCKHBW X9, X9
	MOVO      X1, 32(DI)
	MOVO      X9, 48(DI)
	ADDQ      $32, SI
	ADDQ      $64, DI
	SUBQ      $32, BX
	DECQ      AX
	JNZ       u8toi16_aligned_loop

	// TODO: work smaller sizes of blocks

u8toi16_tail:
	// Single step anything that is left
	ANDQ BX, BX
	JZ   u8toi16_done

u8toi16_tail_loop:
	MOVBQZX (SI), CX
	INCQ    SI
	SUBQ    $128, CX
	MOVB    CX, (DI)
	INCQ    DI
	MOVB    CX, (DI)
	INCQ    DI
	DECQ    BX
	JNZ     u8toi16_tail_loop

u8toi16_done:
	RET

TEXT ·u8ToI16LEAsm(SB), NOSPLIT, $0-48
	MOVQ dst_len+8(FP), CX
	SHRQ $1, CX
	MOVQ dst+0(FP), DI
	MOVQ DI, AX
	ANDQ $1, AX
	JNZ  u8toi16le_unaligned

	// Aligned version can just use U8ToI16 but with adjusted output length.
	MOVQ CX, dst_len+8(FP)
	JMP  ·u8ToI16Asm(SB)

u8toi16le_unaligned:
	// Output is on an odd address which means it cannot be aligned
	MOVQ src+24(FP), SI
	MOVQ src_len+32(FP), BX

	// Choose the shortest length
	CMPQ CX, BX
	JGE  u8toi16le_min_length
	MOVQ CX, BX

u8toi16le_min_length:
	// BX = length

	MOVQ      $0x8080808080808080, AX
	MOVQ      AX, X8
	PUNPCKLBW X8, X8
	MOVQ      BX, AX
	SHRQ      $5, AX
	JZ        u8toi16le_tail

u8toi16le_aligned_loop:
	MOVOU     (SI), X0
	PSUBB     X8, X0
	MOVOU     16(SI), X1
	PSUBB     X8, X1
	MOVO      X0, X9
	PUNPCKLBW X0, X0
	PUNPCKHBW X9, X9
	MOVOU     X0, (DI)
	MOVOU     X9, 16(DI)
	MOVO      X1, X9
	PUNPCKLBW X1, X1
	PUNPCKHBW X9, X9
	MOVOU     X1, 32(DI)
	MOVOU     X9, 48(DI)
	ADDQ      $32, SI
	ADDQ      $64, DI
	SUBQ      $32, BX
	DECQ      AX
	JNZ       u8toi16le_aligned_loop

	// TODO: work increasingly smaller blocks

u8toi16le_tail:
	// Single step anything that is left
	ANDQ BX, BX
	JZ   u8toi16le_done

u8toi16le_tail_loop:
	MOVBQZX (SI), CX
	INCQ    SI
	SUBQ    $128, CX
	MOVB    CX, (DI)
	INCQ    DI
	MOVB    CX, (DI)
	INCQ    DI
	DECQ    BX
	JNZ     u8toi16le_tail_loop

u8toi16le_done:
	RET

//TEXT ·U8ToF32(SB), NOSPLIT, $0-48
//	MOVQ src+24(FP), SI
//	MOVQ src_len+32(FP), AX
//	MOVQ dst+0(FP), DI
//	MOVQ dst_len+8(FP), CX
//
//	CMPQ AX, CX
//	JGE  u8tof32_min_len
//	MOVQ AX, CX
//
//u8tof32_min_len:
//
//	MOVQ $0, AX
//
//	// Too short to optimize
//	MOVQ $32, BX
//	CMPQ BX, CX
//	JGE  u8tof32_stepper
//
//	// Align output to 16-byte boundary
//	MOVQ DI, BP
//	ANDQ $15, BP
//	SHRQ $2, BP
//	JZ   u8tof32_aligned
//	MOVQ $4, DX
//	SUBQ BP, DX
//
//u8tof32_align:
//	MOVBQZX  (SI), BX
//	INCQ     SI
//	SUBQ     $128, BX
//	CVTSQ2SS BX, X0
//	MOVSS    X0, (DI)
//	ADDQ     $4, DI
//	INCQ     AX
//	DECQ     DX
//	JNZ      u8tof32_align
//
//u8tof32_aligned:
//
//	MOVQ CX, DX
//	ANDQ $(~15), DX
//	CMPQ AX, DX
//	JGE  u8tof32_stepper
//
//	CMPB ·x86+const_offsetX86HasSSE41(SB), $1
//	JNE  u8tof32_nosse4
//
//	MOVQ   $0, BP
//	MOVL   $0x80808080, BX
//	VMOVD  BX, X8
//	PSHUFL $0, X8, X8
//
//u8tof32_sse4_loop:
//	MOVOU (SI), X0 // Load 16 unsigned 8-bit values
//	PSUBB X8, X0   // Make the values signed
//
//	// Lowest 4 values (bytes 0-3)
//	PMOVSXBD X0, X2
//	CVTPL2PS X2, X2   // Convert 32-bit signed integers to 32-bit float
//	MOVAPS   X2, (DI)
//
//	// Next 4 values (bytes 4-7)
//	PSHUFL   $1, X0, X2
//	PMOVSXBD X2, X2
//	CVTPL2PS X2, X2     // Convert 32-bit signed integers to 32-bit float
//	MOVAPS   X2, 16(DI)
//
//	// Next 4 values (bytes 8-11)
//	PSHUFL   $2, X0, X2
//	PMOVSXBD X2, X2
//	CVTPL2PS X2, X2     // Convert 32-bit signed integers to 32-bit float
//	MOVAPS   X2, 32(DI)
//
//	// Next 4 values (bytes 12-15)
//	PSHUFL   $3, X0, X2
//	PMOVSXBD X2, X2
//	CVTPL2PS X2, X2     // Convert 32-bit signed integers to 32-bit float
//	MOVAPS   X2, 48(DI)
//
//	ADDQ $16, AX
//	ADDQ $16, SI
//	ADDQ $64, DI
//	CMPQ AX, DX
//	JLT  u8tof32_sse4_loop
//	JMP  u8tof32_stepper
//
//u8tof32_nosse4:
//	MOVQ   $0, BP
//	MOVL   $0x80808080, BX
//	VMOVD  BX, X8
//	PSHUFL $0, X8, X8
//
//u8tof32_sse2_loop:
//	MOVOU (SI), X0 // Load 16 unsigned 8-bit values
//	PSUBB X8, X0   // Make the values signed
//	MOVO  X0, X1
//
//	// Lowest 4 values (bytes 0-3)
//	PUNPCKLBW X1, X1
//	MOVO      X1, X2
//	PUNPCKLWL X1, X1
//	PSRAL     $24, X1
//	CVTPL2PS  X1, X1
//	MOVAPS    X1, (DI)
//
//	// Next 4 values (bytes 4-7)
//	PUNPCKHWL X2, X2
//	PSRAL     $24, X2
//	CVTPL2PS  X2, X2
//	MOVAPS    X2, 16(DI)
//
//	// Next 4 values (bytes 8-11)
//	PUNPCKHBW X0, X0
//	MOVO      X0, X2
//	PUNPCKLWL X0, X0
//	PSRAL     $24, X0
//	CVTPL2PS  X0, X0
//	MOVAPS    X0, 32(DI)
//
//	// Next 4 values (bytes 12-15)
//	PUNPCKHWL X2, X2
//	PSRAL     $24, X2
//	CVTPL2PS  X2, X2
//	MOVAPS    X2, 48(DI)
//
//	ADDQ $16, AX
//	ADDQ $16, SI
//	ADDQ $64, DI
//	CMPQ AX, DX
//	JLT  u8tof32_sse2_loop
//
//	// TODO: work increasingly smaller blocks
//
//u8tof32_stepper:
//	CMPQ AX, CX
//	JGE  u8tof32_done
//
//u8tof32_step:
//	MOVBQZX  (SI), BX
//	INCQ     SI
//	SUBQ     $128, BX
//	CVTSQ2SS BX, X0
//	MOVSS    X0, (DI)
//	ADDQ     $4, DI
//	INCQ     AX
//	CMPQ     AX, CX
//	JLT      u8tof32_step
//
//u8tof32_done:
//	RET

TEXT ·i8ToF32Asm(SB), NOSPLIT, $0-48
	MOVQ src+24(FP), SI
	MOVQ src_len+32(FP), AX
	MOVQ dst+0(FP), DI
	MOVQ dst_len+8(FP), CX

	CMPQ AX, CX
	JGE  i8tof32_min_len
	MOVQ AX, CX

i8tof32_min_len:

	MOVQ $0, AX

	// Too short to optimize
	MOVQ $32, BX
	CMPQ BX, CX
	JGE  i8tof32_stepper

	// Align output to 16-byte boundary. R9 is how many float32 slots dst
	// already sits past one, so 4-R9 is how many to step first.
	MOVQ DI, R9
	ANDQ $15, R9
	SHRQ $2, R9
	JZ   i8tof32_aligned
	MOVQ $4, DX
	SUBQ R9, DX

i8tof32_align:
	MOVBQSX  (SI), BX
	INCQ     SI
	CVTSQ2SS BX, X0
	MOVSS    X0, (DI)
	ADDQ     $4, DI
	INCQ     AX
	DECQ     DX
	JNZ      i8tof32_align

i8tof32_aligned:

	// The block bound is relative to the index the prologue left behind, not
	// absolute: an absolute CX&^15 lets the last block start up to 15 elements
	// short of the end and run 15 past it.
	MOVQ CX, DX
	SUBQ AX, DX
	ANDQ $(~15), DX
	ADDQ AX, DX
	CMPQ AX, DX
	JGE  i8tof32_stepper

	// CMPB ·x86+const_offsetX86HasSSE41(SB), $1
	CMPB ·useSSE4(SB), $1
	JNE  i8tof32_nosse4

	MOVQ $0, R9
	MOVQ R9, X9

i8tof32_sse4_loop:
	MOVOU (SI), X0 // Load 16 unsigned 8-bit values

	// Lowest 4 values (bytes 0-3)
	PMOVSXBD X0, X2
	CVTPL2PS X2, X2   // Convert 32-bit signed integers to 32-bit float
	MOVAPS   X2, (DI)

	// Next 4 values (bytes 4-7)
	PSHUFL   $1, X0, X2
	PMOVSXBD X2, X2
	CVTPL2PS X2, X2     // Convert 32-bit signed integers to 32-bit float
	MOVAPS   X2, 16(DI)

	// Next 4 values (bytes 8-11)
	PSHUFL   $2, X0, X2
	PMOVSXBD X2, X2
	CVTPL2PS X2, X2     // Convert 32-bit signed integers to 32-bit float
	MOVAPS   X2, 32(DI)

	// Next 4 values (bytes 12-15)
	PSHUFL   $3, X0, X2
	PMOVSXBD X2, X2
	CVTPL2PS X2, X2     // Convert 32-bit signed integers to 32-bit float
	MOVAPS   X2, 48(DI)

	ADDQ $16, AX
	ADDQ $16, SI
	ADDQ $64, DI
	CMPQ AX, DX
	JLT  i8tof32_sse4_loop
	JMP  i8tof32_stepper

i8tof32_nosse4:
	MOVQ $0, R9
	MOVQ R9, X9

i8tof32_sse2_loop:
	MOVOU (SI), X0 // Load 16 unsigned 8-bit values
	MOVO  X0, X1

	// Lowest 4 values (bytes 0-3)
	PUNPCKLBW X1, X1
	MOVO      X1, X2
	PUNPCKLWL X1, X1
	PSRAL     $24, X1
	CVTPL2PS  X1, X1
	MOVAPS    X1, (DI)

	// Next 4 values (bytes 4-7)
	PUNPCKHWL X2, X2
	PSRAL     $24, X2
	CVTPL2PS  X2, X2
	MOVAPS    X2, 16(DI)

	// // Next 4 values (bytes 8-11)
	PUNPCKHBW X0, X0
	MOVO      X0, X2
	PUNPCKLWL X0, X0
	PSRAL     $24, X0
	CVTPL2PS  X0, X0
	MOVAPS    X0, 32(DI)

	// Next 4 values (bytes 12-15)
	PUNPCKHWL X2, X2
	PSRAL     $24, X2
	CVTPL2PS  X2, X2
	MOVAPS    X2, 48(DI)

	ADDQ $16, AX
	ADDQ $16, SI
	ADDQ $64, DI
	CMPQ AX, DX
	JLT  i8tof32_sse2_loop

	// TODO: work increasingly smaller blocks

i8tof32_stepper:
	CMPQ AX, CX
	JGE  i8tof32_done

i8tof32_step:
	MOVBQSX  (SI), BX
	INCQ     SI
	CVTSQ2SS BX, X0
	MOVSS    X0, (DI)
	ADDQ     $4, DI
	INCQ     AX
	CMPQ     AX, CX
	JLT      i8tof32_step

i8tof32_done:
	RET

// The int16 rails as float32, to clip against before converting. PACKSSLW
// below saturates int32 to int16 on its own, but only after CVTTPS2PL has
// mapped everything outside the int32 range -- and NaN -- to INT32_MIN, which
// packs to the wrong rail for a large positive sample. Clipping in the float
// domain first makes the pack exact.
DATA  i16MinF32<>+0(SB)/4, $0xc7000000 // -32768.0
GLOBL i16MinF32<>(SB), RODATA|NOPTR, $4
DATA  i16MaxF32<>+0(SB)/4, $0x46fffe00 // 32767.0
GLOBL i16MaxF32<>(SB), RODATA|NOPTR, $4

TEXT ·f32ToI16Asm(SB), NOSPLIT, $0-48
	MOVQ   src+24(FP), SI
	MOVQ   src_len+32(FP), AX
	MOVQ   dst+0(FP), DI
	MOVQ   dst_len+8(FP), CX
	MOVSS  i16MinF32<>(SB), X9
	PSHUFD $0, X9, X9
	MOVSS  i16MaxF32<>(SB), X10
	PSHUFD $0, X10, X10

	CMPQ AX, CX
	JGE  f32toI16_min_len
	MOVQ AX, CX

f32toI16_min_len:

	MOVQ CX, DX
	ANDQ $(~31), CX

	MOVQ $0, AX
	CMPQ AX, CX
	JGE  f32toI16_stepper

f32toI16_loop:
	MOVUPS    (SI), X0
	MOVUPS    16(SI), X1
	MOVUPS    32(SI), X2
	MOVUPS    48(SI), X3
	LEAQ      (DI)(AX*2), BX

	// A lane compares equal to itself unless it is NaN, so ANDing with the
	// mask leaves every ordered lane alone and turns NaN into +0.0. The rails
	// are then clipped with no NaN left for MAXPS/MINPS to mishandle -- both
	// of them return their second source operand when either is NaN, so the
	// mask has to come first.
	//
	// This is twenty instructions per sixteen samples on top of a body that
	// was twenty, and it does roughly halve the throughput of this loop: 1.9us
	// against 3.9us for 16K samples on a Xeon Silver 4210. It is still 4 GB/s,
	// which is three orders of magnitude more than an audio or SDR stream
	// needs, and it buys the same answer on all four architectures for every
	// input including NaN. The alternative -- one MINPS, letting PACKSSLW
	// saturate the low rail and NaN land on whichever rail it falls on -- is
	// four instructions instead of twenty but makes NaN architecture
	// dependent, since the ARM converts give zero for it.
	MOVAPS    X0, X4
	MOVAPS    X1, X5
	MOVAPS    X2, X6
	MOVAPS    X3, X7
	CMPPS     X4, X4, $0
	CMPPS     X5, X5, $0
	CMPPS     X6, X6, $0
	CMPPS     X7, X7, $0
	ANDPS     X4, X0
	ANDPS     X5, X1
	ANDPS     X6, X2
	ANDPS     X7, X3
	MAXPS     X9, X0
	MAXPS     X9, X1
	MAXPS     X9, X2
	MAXPS     X9, X3
	MINPS     X10, X0
	MINPS     X10, X1
	MINPS     X10, X2
	MINPS     X10, X3

	CVTTPS2PL X0, X0
	CVTTPS2PL X1, X1
	CVTTPS2PL X2, X2
	CVTTPS2PL X3, X3
	PACKSSLW  X1, X0
	PACKSSLW  X3, X2
	MOVOU     X0, (BX)
	MOVOU     X2, 16(BX)
	ADDQ      $64, SI
	ADDQ      $16, AX
	CMPQ      AX, CX
	JLT       f32toI16_loop

f32toI16_stepper:
	CMPQ AX, DX
	JGE  f32toI16_done

f32toI16_step:
	MOVSS     (SI), X0
	LEAQ      (DI)(AX*2), BX
	MOVAPS    X0, X4
	CMPPS     X4, X4, $0
	ANDPS     X4, X0
	MAXSS     X9, X0
	MINSS     X10, X0
	CVTTSS2SL X0, R9
	MOVW      R9, (BX)
	ADDQ      $4, SI
	INCQ      AX
	CMPQ      AX, DX
	JLT       f32toI16_step

f32toI16_done:
	RET

TEXT ·f32ToI16LEAsm(SB), NOSPLIT, $0-48
	MOVQ dst_len+8(FP), AX
	SHRQ $1, AX
	MOVQ AX, dst_len+8(FP)
	JMP  ·f32ToI16Asm(SB)

TEXT ·i16ToI16LEAsm(SB), NOSPLIT, $0-48
	JMP ·i16ToI16LE(SB)

TEXT ·i16LEToF64Asm(SB), NOSPLIT, $0-48
	MOVQ src+24(FP), SI
	MOVQ src_len+32(FP), CX
	MOVQ dst+0(FP), DI
	MOVQ dst_len+8(FP), AX

	SARQ $1, CX
	CMPQ CX, AX
	JLT  i16leToF64_min_len
	MOVQ AX, CX

i16leToF64_min_len:
	// CX = min length in samples

	MOVQ $0, BX
	MOVQ CX, DX

	//CMPB ·x86+const_offsetX86HasSSE41(SB), $1
	CMPB ·useSSE4(SB), $1
	JNE  i16leToF64_nosse4

	ANDQ $-8, DX

i16leToF64_sse4_loop:
	CMPQ     BX, DX
	JGE      i16leToF64_scalar_loop
	MOVOU    (SI), X1                // Load 8 16-bit integers (0..15, 16..31, 32..47, 48..63)
	PMOVSXWD X1, X2                  // SSE4.1 Sign-extend 16-bit integers to 32-bit integers (0..31, 32..63, 64..95, 96..127)
	CVTPL2PD X2, X3                  // Convert 32-bit signed integers to 64-bit float
	MOVHLPS  X2, X2                  // Move 64..127 to 0..63
	CVTPL2PD X2, X2                  // Convert 32-bit signed integers to 64-bit float
	MOVHLPS  X1, X1                  // Move 64..127 to 0..63
	PMOVSXWD X1, X4                  // SSE4.1 Sign-extend 16-bit integers to 32-bit integers (0..31, 32..63, 64..95, 96..127)
	CVTPL2PD X4, X5                  // Convert 32-bit signed integers to 64-bit float
	MOVHLPS  X4, X4                  // Move 64..127 to 0..63
	CVTPL2PD X4, X4                  // Convert 32-bit signed integers to 64-bit float
	MOVUPD   X3, (DI)
	MOVUPD   X2, 16(DI)
	MOVUPD   X5, 32(DI)
	MOVUPD   X4, 48(DI)
	ADDQ     $16, SI
	ADDQ     $64, DI
	ADDQ     $8, BX
	JMP      i16leToF64_sse4_loop

i16leToF64_nosse4:
	ANDQ $-4, DX

i16leToF64_sse2_loop:
	CMPQ      BX, DX
	JGE       i16leToF64_scalar_loop
	MOVQ      (SI), X1                // Load 4 16-bit integers (0..15, 16..31, 32..47, 48..63)
	PUNPCKLWL X1, X1
	PSRAL     $16, X1
	CVTPL2PD  X1, X3                  // Convert 32-bit signed integers to 64-bit float
	MOVHLPS   X1, X1                  // Move 64..127 to 0..63
	CVTPL2PD  X1, X1                  // Convert 32-bit signed integers to 64-bit float
	MOVUPD    X3, (DI)
	MOVUPD    X1, 16(DI)
	ADDQ      $8, SI
	ADDQ      $32, DI
	ADDQ      $4, BX
	JMP       i16leToF64_sse2_loop

i16leToF64_scalar_loop:
	CMPQ     BX, CX
	JGE      i16leToF64_done
	MOVWLSX  (SI), DX
	XORPS    X1, X1
	CVTSL2SD DX, X1
	MOVSD    X1, (DI)
	ADDQ     $2, SI
	ADDQ     $8, DI
	INCQ     BX
	JMP      i16leToF64_scalar_loop

i16leToF64_done:
	RET

TEXT ·i16LEToF32Asm(SB), NOSPLIT, $0-48
	MOVQ   src+24(FP), SI
	MOVQ   src_len+32(FP), CX
	MOVQ   dst+0(FP), DI
	MOVQ dst_len+8(FP), AX

	SARQ $1, CX
	CMPQ CX, AX
	JLT  i16leToF32_min_len
	MOVQ AX, CX

i16leToF32_min_len:
	// CX = min length in samples

	MOVQ $0, BX
	MOVQ CX, DX

	//CMPB ·x86+const_offsetX86HasSSE41(SB), $1
	CMPB ·useSSE4(SB), $1
	JNE  i16leToF32_nosse4

	ANDQ $-8, DX

i16leToF32_sse4_loop:
	CMPQ     BX, DX
	JGE      i16leToF32_scalar_loop
	MOVOU    (SI), X1                // Load 8 16-bit integers (0..15, 16..31, 32..47, 48..63)
	PMOVSXWD X1, X2                  // SSE4.1 Sign-extend 16-bit integers to 32-bit integers (0..31, 32..63, 64..95, 96..127)
	CVTPL2PS X2, X2                  // Convert 32-bit signed integers to 32-bit float X2:0..63=X2:0..31, X2:64..127=X2:32..63
	MOVHLPS  X1, X1                  // Move 64..127 to 0..63
	PMOVSXWD X1, X1                  // SSE4.1 Sign-extend 16-bit integers to 32-bit integers (0..31, 32..63, 64..95, 96..127)
	CVTPL2PS X1, X1                  // Convert 32-bit signed integers to 32-bit float X2:0..63=X2:0..31, X2:64..127=X2:32..63
	MOVUPS   X2, (DI)
	MOVUPS   X1, 16(DI)
	ADDQ     $16, SI
	ADDQ     $32, DI
	ADDQ     $8, BX
	JMP      i16leToF32_sse4_loop

i16leToF32_nosse4:
	ANDQ $-8, DX

i16leToF32_sse2_loop:
	CMPQ      BX, DX
	JGE       i16leToF32_scalar_loop
	MOVOU     (SI), X2                // Load 8 16-bit integers (0..15, 16..31, 32..47, 48..63)
	MOVO      X2, X1
	PUNPCKLWL X1, X1
	PSRAL     $16, X1
	CVTPL2PS  X1, X1                  // Convert 32-bit signed integers to 32-bit float
	MOVHLPS   X2, X2                  // Move 64..127 to 0..63
	PUNPCKLWL X2, X2
	PSRAL     $16, X2
	CVTPL2PS  X2, X2                  // Convert 32-bit signed integers to 32-bit float
	MOVUPS    X1, (DI)
	MOVUPS    X2, 16(DI)
	ADDQ      $16, SI
	ADDQ      $32, DI
	ADDQ      $8, BX
	JMP       i16leToF32_sse2_loop

i16leToF32_scalar_loop:
	CMPQ     BX, CX
	JGE      i16leToF32_done
	MOVWLSX  (SI), DX
	XORPS    X1, X1
	CVTSL2SS DX, X1
	MOVSS    X1, (DI)
	ADDQ     $2, SI
	ADDQ     $4, DI
	INCQ     BX
	JMP      i16leToF32_scalar_loop

i16leToF32_done:
	RET
