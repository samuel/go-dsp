
TEXT ·FastAtan2(SB),7,$0
	JMP ·fastAtan2(SB)

// 	MOVSS	y+0(FP), X1
// 	MOVSS	x+4(FP), X4

// 	MOVQ	$(1<<31), BX
// 	MOVQ	BX, X3
// 	MOVSS	X3, X5

// 	// abs(y) + 1.0e-10
// 	ANDNPS	X1, X3
// 	ADDSS	$1.0e-10, X3

// 	MOVSS	X4, X2

// 	MOVSS	$0.0, X0
// 	UCOMISS	X0, X4
// 	JHI	L4

// 	ADDSS	X3, X2	// abs(y) + x
// 	MOVSS	X3, X0
// 	SUBSS	X4, X0	// abs(y) - x
// 	DIVSS	X0, X2
// 	MOVSS	$2.356194496154785, X3
// 	JMP	L5
// L4:
// 	SUBSS	X3, X2	// x - abs(y)
// 	ADDSS	X3, X4	// x + abs(y)
// 	DIVSS	X4, X2
// 	MOVSS	$0.7853981852531433, X3
// L5:

// 	MOVSS	$0.1963, X0
// 	MULSS	X2, X0
// 	MULSS	X2, X0
// 	SUBSS	$0.9817, X0
// 	MULSS	X2, X0
// 	ADDSS	X3, X0

// 	// if x < 0: -angle
// 	ANDPS	X1, X5
// 	XORPS	X5, X0

// 	MOVSS	X0, ret+8(FP)
// 	RET
