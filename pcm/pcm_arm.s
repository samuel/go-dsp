#include "textflag.h"

// Each kernel runs a NEON loop when ·haveNEON is set and a VFP loop
// otherwise, then finishes in a scalar VFP loop. The Go assembler has no
// mnemonics for NEON, SSAT, SXTB or most VFP operations here, so they are
// encoded with WORD, each followed by the instruction it is.
//
// Every loop loads bytes with byte-granular VLD1 or LDRB, so src needs no
// alignment, and stores floats with VST1.32 or VSTM, which needs only the
// four-byte alignment a []float32 has.
//
// NEON arithmetic always rounds to nearest even and flushes denormals to zero.
// Neither changes a result here: decoding multiplies small integers by a power
// of two, and encoding flushes only inputs that round to zero anyway.

// The decode scales, 2^-7 and 2^-15, as float32 bits.
#define SCALE8 $0x3c000000
#define SCALE16 $0x38000000

// CVTMUL8 converts the int32 lanes of q1-q8 to float32, multiplies them by
// q9 and stores them at R3, advancing it by 128 bytes: vcvt.f32.s32 qN, qN
// for q1-q8, vmul.f32 qN, qN, q9 for q1-q8, then vst1.32 {d2-d5}, {d6-d9},
// {d10-d13} and {d14-d17}, [r3]!.
#define CVTMUL8 \
	WORD $0xf3bb2642; \
	WORD $0xf3bb4644; \
	WORD $0xf3bb6646; \
	WORD $0xf3bb8648; \
	WORD $0xf3bba64a; \
	WORD $0xf3bbc64c; \
	WORD $0xf3bbe64e; \
	WORD $0xf3fb0660; \
	WORD $0xf3022d72; \
	WORD $0xf3044d72; \
	WORD $0xf3066d72; \
	WORD $0xf3088d72; \
	WORD $0xf30aad72; \
	WORD $0xf30ccd72; \
	WORD $0xf30eed72; \
	WORD $0xf3400df2; \
	WORD $0xf403228d; \
	WORD $0xf403628d; \
	WORD $0xf403a28d; \
	WORD $0xf403e28d;

// CVTMUL1 converts the int32 in R4 to float32, multiplies it by s8 and
// stores it at R3, advancing it by 4 bytes: vmov s0, r4; vcvt.f32.s32 s0, s0;
// vmul.f32 s0, s0, s8; vstmia r3!, {s0}.
#define CVTMUL1 \
	WORD $0xee004a10; \
	WORD $0xeeb80ac0; \
	WORD $0xee200a04; \
	WORD $0xeca30a01;

// func u8ToF32(dst []float32, src []byte)
TEXT ·u8ToF32(SB), NOSPLIT, $0-24
	MOVW src_base+12(FP), R1
	MOVW src_len+16(FP), R2
	MOVW dst_base+0(FP), R3
	MOVW dst_len+4(FP), R0

	// R2 = min(len(dst), len(src)), the sample count
	CMP     R2, R0
	MOVW.LT R0, R2
	CMP     $0, R2
	BEQ     u8tof32_done

	MOVW SCALE8, R5
	WORD $0xee045a10 // vmov s8, r5

	MOVBU ·haveNEON+0(SB), R0
	CMP   $0, R0
	BNE   u8tof32_neon

	// VFP: four samples per word. USUB8 removes the bias from all four bytes
	// at once, leaving each a signed byte for SXTB.
	AND $(~3), R2, R0
	ADD R1, R2
	CMP $0, R0
	BEQ u8tof32_tail
	ADD R1, R0

	MOVW $0x80808080, R8

u8tof32_loop:
	MOVW (R1), R4
	ADD  $4, R1
	WORD $0xe6544ff8 // usub8 r4, r4, r8
	WORD $0xe6af5474 // sxtb r5, r4, ror #8
	WORD $0xe6af6874 // sxtb r6, r4, ror #16
	WORD $0xe6af7c74 // sxtb r7, r4, ror #24
	WORD $0xe6af4074 // sxtb r4, r4
	WORD $0xec454a1e // vmov s28, s29, r4, r5
	WORD $0xec476a1f // vmov s30, s31, r6, r7
	WORD $0xeeb80ace // vcvt.f32.s32 s0, s28
	WORD $0xeef80aee // vcvt.f32.s32 s1, s29
	WORD $0xeeb81acf // vcvt.f32.s32 s2, s30
	WORD $0xeef81aef // vcvt.f32.s32 s3, s31
	WORD $0xee200a04 // vmul.f32 s0, s0, s8
	WORD $0xee600a84 // vmul.f32 s1, s1, s8
	WORD $0xee211a04 // vmul.f32 s2, s2, s8
	WORD $0xee611a84 // vmul.f32 s3, s3, s8
	WORD $0xeca30a04 // vstmia r3!, {s0-s3}
	CMP  R0, R1
	BLT  u8tof32_loop
	B    u8tof32_tail

	// NEON: 32 samples per iteration, widened to 16 and then 32 bits.
u8tof32_neon:
	MOVW $128, R0
	WORD $0xeee00b10 // vdup.8 q0, r0
	WORD $0xeea25b90 // vdup.32 q9, r5

	AND $(~31), R2, R4
	ADD R1, R2
	CMP $0, R4
	BEQ u8tof32_tail
	ADD R1, R4

u8tof32_neon_loop:
	WORD $0xf461c20d // vld1.8 {d28-d31}, [r1]!
	WORD $0xf3cc4280 // vsubl.u8 q10, d28, d0
	WORD $0xf3cd6280 // vsubl.u8 q11, d29, d0
	WORD $0xf3ce8280 // vsubl.u8 q12, d30, d0
	WORD $0xf3cfa280 // vsubl.u8 q13, d31, d0
	WORD $0xf2902a34 // vmovl.s16 q1, d20
	WORD $0xf2904a35 // vmovl.s16 q2, d21
	WORD $0xf2906a36 // vmovl.s16 q3, d22
	WORD $0xf2908a37 // vmovl.s16 q4, d23
	WORD $0xf290aa38 // vmovl.s16 q5, d24
	WORD $0xf290ca39 // vmovl.s16 q6, d25
	WORD $0xf290ea3a // vmovl.s16 q7, d26
	WORD $0xf2d00a3b // vmovl.s16 q8, d27
	CVTMUL8
	CMP  R4, R1
	BLT  u8tof32_neon_loop
	WORD $0xee045a10 // vmov s8, r5, which the NEON loop overwrote

u8tof32_tail:
	CMP R1, R2
	BEQ u8tof32_done

u8tof32_tail_loop:
	MOVBU 0(R1), R4
	ADD   $1, R1
	SUB   $128, R4
	CVTMUL1
	CMP   R2, R1
	BLT   u8tof32_tail_loop

u8tof32_done:
	RET

// func i8ToF32(dst []float32, src []byte)
TEXT ·i8ToF32(SB), NOSPLIT, $0-24
	MOVW src_base+12(FP), R1
	MOVW src_len+16(FP), R2
	MOVW dst_base+0(FP), R3
	MOVW dst_len+4(FP), R0

	CMP     R2, R0
	MOVW.LT R0, R2
	CMP     $0, R2
	BEQ     i8tof32_done

	MOVW SCALE8, R5
	WORD $0xee045a10 // vmov s8, r5

	MOVBU ·haveNEON+0(SB), R0
	CMP   $0, R0
	BEQ   i8tof32_novec

	WORD $0xeea25b90 // vdup.32 q9, r5

	AND $(~31), R2, R4
	ADD R1, R2
	CMP $0, R4
	BEQ i8tof32_tail
	ADD R1, R4

i8tof32_neon_loop:
	WORD $0xf461c20d // vld1.8 {d28-d31}, [r1]!
	WORD $0xf2c84a3c // vmovl.s8 q10, d28
	WORD $0xf2c86a3d // vmovl.s8 q11, d29
	WORD $0xf2c88a3e // vmovl.s8 q12, d30
	WORD $0xf2c8aa3f // vmovl.s8 q13, d31
	WORD $0xf2902a34 // vmovl.s16 q1, d20
	WORD $0xf2904a35 // vmovl.s16 q2, d21
	WORD $0xf2906a36 // vmovl.s16 q3, d22
	WORD $0xf2908a37 // vmovl.s16 q4, d23
	WORD $0xf290aa38 // vmovl.s16 q5, d24
	WORD $0xf290ca39 // vmovl.s16 q6, d25
	WORD $0xf290ea3a // vmovl.s16 q7, d26
	WORD $0xf2d00a3b // vmovl.s16 q8, d27
	CVTMUL8
	CMP  R4, R1
	BLT  i8tof32_neon_loop
	WORD $0xee045a10 // vmov s8, r5, which the NEON loop overwrote
	B    i8tof32_tail

i8tof32_novec:
	ADD R1, R2

i8tof32_tail:
	CMP R1, R2
	BEQ i8tof32_done

i8tof32_tail_loop:
	MOVB 0(R1), R4
	ADD  $1, R1
	CVTMUL1
	CMP  R2, R1
	BLT  i8tof32_tail_loop

i8tof32_done:
	RET

// func i16LEToF32(dst []float32, src []byte)
TEXT ·i16LEToF32(SB), NOSPLIT, $0-24
	MOVW src_base+12(FP), R1
	MOVW src_len+16(FP), R2
	MOVW dst_base+0(FP), R3
	MOVW dst_len+4(FP), R0

	// R2 = min(len(dst), len(src)/2)
	MOVW    R2>>1, R2
	CMP     R2, R0
	MOVW.LT R0, R2
	CMP     $0, R2
	BEQ     i16tof32_done

	MOVW SCALE16, R5
	WORD $0xee045a10 // vmov s8, r5

	MOVBU ·haveNEON+0(SB), R0
	CMP   $0, R0
	BEQ   i16tof32_novec

	WORD $0xeea25b90 // vdup.32 q9, r5

	// 16 samples, 32 bytes, per iteration. R4 and R2 become byte bounds.
	AND $(~15), R2, R4
	ADD R2<<1, R1, R2
	CMP $0, R4
	BEQ i16tof32_tail
	ADD R4<<1, R1, R4

i16tof32_neon_loop:
	WORD $0xf461c20d // vld1.8 {d28-d31}, [r1]!
	WORD $0xf2902a3c // vmovl.s16 q1, d28
	WORD $0xf2904a3d // vmovl.s16 q2, d29
	WORD $0xf2906a3e // vmovl.s16 q3, d30
	WORD $0xf2908a3f // vmovl.s16 q4, d31
	WORD $0xf3bb2642 // vcvt.f32.s32 q1, q1
	WORD $0xf3bb4644 // vcvt.f32.s32 q2, q2
	WORD $0xf3bb6646 // vcvt.f32.s32 q3, q3
	WORD $0xf3bb8648 // vcvt.f32.s32 q4, q4
	WORD $0xf3022d72 // vmul.f32 q1, q1, q9
	WORD $0xf3044d72 // vmul.f32 q2, q2, q9
	WORD $0xf3066d72 // vmul.f32 q3, q3, q9
	WORD $0xf3088d72 // vmul.f32 q4, q4, q9
	WORD $0xf403228d // vst1.32 {d2-d5}, [r3]!
	WORD $0xf403628d // vst1.32 {d6-d9}, [r3]!
	CMP  R4, R1
	BLT  i16tof32_neon_loop
	WORD $0xee045a10 // vmov s8, r5, which the NEON loop overwrote
	B    i16tof32_tail

i16tof32_novec:
	ADD R2<<1, R1, R2

i16tof32_tail:
	CMP R1, R2
	BEQ i16tof32_done

	// Two byte loads, since src need not be halfword aligned.
i16tof32_tail_loop:
	MOVBU 0(R1), R4
	MOVB  1(R1), R5
	ADD   $2, R1
	ORR   R5<<8, R4
	CVTMUL1
	CMP   R2, R1
	BLT   i16tof32_tail_loop

i16tof32_done:
	RET

// func f32ToI16LE(dst []byte, src []float32)
//
// Each sample is multiplied by 32768, rounded to nearest even and clipped to
// the int16 range, with NaN giving zero.
TEXT ·f32ToI16LE(SB), NOSPLIT, $0-24
	MOVW src_base+12(FP), R1
	MOVW src_len+16(FP), R2
	MOVW dst_base+0(FP), R3
	MOVW dst_len+4(FP), R0

	// R2 = min(len(dst)/2, len(src))
	MOVW    R0>>1, R0
	CMP     R2, R0
	MOVW.LT R0, R2
	CMP     $0, R2
	BEQ     f32toi16_done

	MOVW $0x47000000, R5 // 32768
	WORD $0xee045a10     // vmov s8, r5

	MOVBU ·haveNEON+0(SB), R0
	CMP   $0, R0
	BEQ   f32toi16_novec

	// NEON has no round-to-nearest convert on ARMv7. The NaN lanes are
	// zeroed through a self-compare mask, the rest clipped, and adding and
	// subtracting 1.5*2^23 rounds them to integers, to nearest even, so that
	// the truncating convert is exact.
	MOVW $0xc7000000, R6 // -32768
	MOVW $0x46fffe00, R7 // 32767
	MOVW $0x4b400000, R8 // 1.5 * 2^23
	WORD $0xeea05b90     // vdup.32 q8, r5
	WORD $0xeea26b90     // vdup.32 q9, r6
	WORD $0xeea47b90     // vdup.32 q10, r7
	WORD $0xeea68b90     // vdup.32 q11, r8

	// Eight samples, 32 bytes, per iteration. R4 and R2 become byte bounds.
	AND $(~7), R2, R4
	ADD R2<<2, R1, R2
	CMP $0, R4
	BEQ f32toi16_tail
	ADD R4<<2, R1, R4

f32toi16_neon_loop:
	WORD $0xf421028d // vld1.32 {d0-d3}, [r1]!
	WORD $0xf3000d70 // vmul.f32 q0, q0, q8
	WORD $0xf3022d70 // vmul.f32 q1, q1, q8
	WORD $0xf2004e40 // vceq.f32 q2, q0, q0
	WORD $0xf2026e42 // vceq.f32 q3, q1, q1
	WORD $0xf2000154 // vand q0, q0, q2
	WORD $0xf2022156 // vand q1, q1, q3
	WORD $0xf2000f62 // vmax.f32 q0, q0, q9
	WORD $0xf2022f62 // vmax.f32 q1, q1, q9
	WORD $0xf2200f64 // vmin.f32 q0, q0, q10
	WORD $0xf2222f64 // vmin.f32 q1, q1, q10
	WORD $0xf2000d66 // vadd.f32 q0, q0, q11
	WORD $0xf2022d66 // vadd.f32 q1, q1, q11
	WORD $0xf2200d66 // vsub.f32 q0, q0, q11
	WORD $0xf2222d66 // vsub.f32 q1, q1, q11
	WORD $0xf3bb0740 // vcvt.s32.f32 q0, q0
	WORD $0xf3bb2742 // vcvt.s32.f32 q1, q1
	WORD $0xf3b60200 // vmovn.i32 d0, q0
	WORD $0xf3b61202 // vmovn.i32 d1, q1
	WORD $0xf4030a0d // vst1.8 {d0-d1}, [r3]!
	CMP  R4, R1
	BLT  f32toi16_neon_loop
	WORD $0xee045a10 // vmov s8, r5, which the NEON loop overwrote
	B    f32toi16_tail

f32toi16_novec:
	ADD R2<<2, R1, R2

f32toi16_tail:
	CMP R1, R2
	BEQ f32toi16_done

	// VCVTR rounds by FPSCR, to nearest even, saturates to the int32 range
	// and converts NaN to zero; SSAT then clips to int16. The two byte stores
	// need no alignment.
f32toi16_tail_loop:
	WORD $0xecb10a01 // vldmia r1!, {s0}
	WORD $0xee200a04 // vmul.f32 s0, s0, s8
	WORD $0xeebd0a40 // vcvtr.s32.f32 s0, s0
	WORD $0xee100a10 // vmov r0, s0
	WORD $0xe6af0010 // ssat r0, #16, r0
	MOVB R0, 0(R3)
	MOVW R0>>8, R0
	MOVB R0, 1(R3)
	ADD  $2, R3
	CMP  R2, R1
	BLT  f32toi16_tail_loop

f32toi16_done:
	RET
