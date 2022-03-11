#include "textflag.h"

// TEXT ·Atan2LUT(SB),NOSPLIT,$0
// 	MOVW	x+4(FP), R4
// 	MOVW	y+0(FP), R3
// 	MOVW	$0, R0
// 	TEQ	$0, R4
// 	BNE	L1
// 	CMP	$0, R3
// 	BLE	L2
// 	MOVW	$8192, R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L2:
// 	CMP	$0, R3
// 	BGE	L3
// 	MOVW	$-8192, R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L3:
// 	MOVW	$0, R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L1:
// 	MOVW	R3<<8, R0
// 	MOVW	0(R13), R11
// 	MOVW.W	R11, -8(R13)
// 	MOVW	R4, 4(R13)
// 	MOVW	R0, R11
// 	BL	_div(SB)
// 	MOVW	R11, R2
// 	ADD	$8, R13
// 	TEQ	$0, R2
// 	BNE	L4
// 	CMP	$0, R4
// 	BLE	L5
// 	MOVW	$0,R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L5:
// 	CMP	$0, R3,
// 	BGE	L6
// 	MOVW	$-16384, R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L6:
// 	MOVW	$16384,R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L4:
// 	MOVW	$131072, R4
// 	CMP	R4, R2,
// 	BGE	L7
// 	RSB	$0, R2, R4
// 	MOVW	$131072, R5
// 	CMP	R5, R4
// 	BGE	L7
// 	CMP	$0, R2
// 	BLE	L8
// 	CMP	$0, R3
// 	BLE	L9
// 	MOVW	$atanLUT+0(SB), R0
// // 	MOVW	R2, R1
// // 	MOVW	4(R0), R2
// // 	CMP	R2, R1
// // 	BLO	L10
// // 	PCDATA	$1,$0
// // 	BL	,runtime.panicindex(SB)
// // L10:
// 	MOVW	0(R0), R0
// 	MOVW	R1<<2(R0), R1
// 	MOVW	R1, res+8(FP)
// 	RET
// L9:
// 	MOVW	$atanLUT+0(SB),R0
// // 0x0120 00288 MOVW	R2,R1
// // 0x0124 00292 MOVW	4(R0),R2
// // 0x0128 00296 CMP	R2,R1,
// // 0x012c 00300 BLO	,312
// // 0x0130 00304 PCDATA	$1,$0
// // 0x0130 00304 BL	,runtime.panicindex(SB)
// // 0x0134 00308 UNDEF	,
// 	MOVW	0(R0),R0
// 	MOVW	R1<<2(R0),R0
// 	SUB	$16384,R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L8:
// 	CMP	$0, R3,
// 	BLE	L10
// 	RSB	$0,R2,R1
// 	MOVW	$atanLUT+0(SB),R0
// // 0x015c 00348 MOVW	4(R0),R2
// // 0x0160 00352 CMP	R2,R1,
// // 0x0164 00356 BLO	,368
// // 0x0168 00360 PCDATA	$1,$0
// // 0x0168 00360 BL	,runtime.panicindex(SB)
// // 0x016c 00364 UNDEF	,
// 	MOVW	0(R0),R0
// 	MOVW	R1<<2(R0),R0
// 	MOVW	$16384,R1
// 	SUB	R0,R1
// 	MOVW	R1, res+8(FP)
// 	RET
// L10:
// 	RSB	$0,R2,R1
// 	MOVW	$atanLUT+0(SB),R0
// // 0x0190 00400 MOVW	4(R0),R2
// // 0x0194 00404 CMP	R2,R1,
// // 0x0198 00408 BLO	,420
// // 0x019c 00412 PCDATA	$1,$0
// // 0x019c 00412 BL	,runtime.panicindex(SB)
// // 0x01a0 00416 UNDEF	,
// 	MOVW	0(R0),R0
// 	MOVW	R1<<2(R0),R0
// 	RSB	$0,R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L7:
// 	CMP	$0, R3
// 	BLE	L11
// 	MOVW	$8192,R0
// 	MOVW	R0, res+8(FP)
// 	RET
// L11:
// 	MOVW	$-8192, R0
// 	MOVW	R0, res+8(FP)
// 	RET
// // 0x01dc 00476 WORD	,$-8192
// // 0x01e0 00480 WORD	,$-16384
// // 0x01e4 00484 WORD	,$"".atanLUT+0(SB)

// TEXT ·FastAtan2Fixed(SB),NOSPLIT,$0-12
// 	MOVW	y+0(FP), R5
// 	MOVW	x+4(FP), R4
// 	TEQ	$0, R4
// 	BNE	fatan2fixed_1
// 	TEQ	$0, R5
// 	BNE	fatan2fixed_1
// 	MOVW	R4, res+8(FP)
// 	RET
// fatan2fixed_1:
// 	MOVW	R5, R3 // yAbs = y
// 	CMP	$0, R5
// 	RSB.LT	$0, R3, R3 // if yAbs < 0 : yAbs = -yAbs
// 	CMP	$0, R4
// 	BLT	fatan2fixed_2

// 	SUB	R3, R4, R11
// 	MOVW	R11<<12, R11
// 	ADD	R3, R4, R1

// 	MOVW	0(R13), R0
// 	MOVW.W	R0, -8(R13)
// 	MOVW	R1, 4(R13)
// 	BL	_div(SB)
// 	ADD	$8, R13

// 	MOVW	$4096, R1
// 	SUB	R11, R1, R2
// 	CMP	$0, R5
// 	RSB.LT	$0, R2, R2
// 	MOVW	R2, res+8(FP)
// 	RET
// fatan2fixed_2:
// 	ADD	R3, R4, R11
// 	MOVW	R11<<12, R11
// 	SUB	R4, R3, R1

// 	MOVW	0(R13), R0
// 	MOVW.W	R0, -8(R13)
// 	MOVW	R1, 4(R13)
// 	BL	_div(SB)
// 	ADD	$8, R13

// 	MOVW	$12288, R1
// 	SUB	R11, R1, R2
// 	CMP	$0, R5
// 	RSB.LT	$0, R2, R2
// 	MOVW	R2, res+8(FP)
// 	RET

