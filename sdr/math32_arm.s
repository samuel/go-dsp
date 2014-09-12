
// Uses F0, F1, F2, F3, F4, F6
TEXT ·FastAtan2(SB),7,$-4
	MOVF	y+0(FP), F6
	MOVF	x+4(FP), F4

	ABSF	F6, F2

	MOVF	$1e-20, F0
	ADDF	F0, F2

	WORD	$0xeeb54ac0 // vcmpe.f32 s8, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	BGT	fatan2_pos_x
	BEQ	fatan2_zero_x

	ADDF	F2, F4, F1	// x + abs(y)
	SUBF	F4, F2, F4	// abs(y) - x
	MOVF	$2.356194496154785, F3	// pi * 3/4
	B	fatan2_2
fatan2_pos_x:
	SUBF	F2, F4, F1	// x - abs(y)
	ADDF	F2, F4, F4	// abs(y) + x
	MOVF	$0.7853981852531433, F3	// pi * 1/4
fatan2_2:
	DIVF	F4, F1, F2

	MOVF	$0.1963, F1
	MULF	F2, F1
	MULF	F2, F1
	MOVF	$0.9817, F0
	SUBF	F0, F1
	MULF	F2, F1
	ADDF	F3, F1

	WORD	$0xeeb56ac0 // vcmpe.f32 s12, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	WORD 	$0xbeb11a41 // vneglt.f32 s2, s2
	MOVF	F1, ret+8(FP)
	RET
fatan2_zero_x:
	WORD	$0xeeb56ac0 // vcmpe.f32 s12, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	BGT	fatan2_pos_y
	BLT	fatan2_neg_y
	MOVF	F6, res+8(FP)
	RET
fatan2_neg_y:
	MOVF	$-1.570796326794896557998981734272092580795288085938, F6
	MOVF	F6, res+8(FP)
	RET
fatan2_pos_y:
	MOVF	$1.570796326794896557998981734272092580795288085938, F6
	MOVF	F6, res+8(FP)
	RET


// Uses F0, F1, F2, F3, F4, F6
TEXT ·FastAtan2_2(SB),7,$-4
	MOVF	x+4(FP), F6
	MOVF	y+0(FP), F3
	WORD	$0xeeb56ac0 // vcmpe.f32 s12, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	BEQ	fatan22_zero_x

	// y / x
	DIVF	F6, F3, F1
	MULF	F1, F1, F2
	MOVF	$1.0, F0
	CMPF	F0, F2
	BGT	fatan22_5
	// z / (1.0 + 0.28*z*z)
	MOVF	$0.28, F4
	MULF	F4, F2
	ADDF	F0, F2
	DIVF	F2, F1, F2
	WORD	$0xeeb56ac0 // vcmpe.f32 s12, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	BGE	fatan22_6
	MOVF	$3.14159265358979323846264338327950288419716939937510582097494459, F1
	WORD	$0xeeb53ac0 // vcmpe.f32 s6, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	SUBF.LT	F1, F2
	ADDF.GE	F1, F2
fatan22_6:
	MOVF	F2, res+8(FP)
	RET
fatan22_5:
	// pi2 - z/(z*z+0.28)
	MOVF	$0.28, F4
	ADDF	F4, F2
	DIVF	F2, F1, F2
	MOVF	$1.570796326794896557998981734272092580795288085938, F1
	SUBF	F2, F1, F2
	MOVF	$3.14159265358979323846264338327950288419716939937510582097494459, F1
	WORD	$0xeeb53ac0 // vcmpe.f32 s6, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	SUBF.LT	F1, F2
	MOVF	F2, res+8(FP)
	RET

fatan22_zero_x:
	WORD	$0xeeb53ac0 // vcmpe.f32 s6, #0x0
	WORD	$0xeef1fa10 // vmrs APSR_nzcv, fpscr
	// MOVF.LT	$-1.570796326794896557998981734272092580795288085938, F6
	// MOVF.GT	$1.570796326794896557998981734272092580795288085938, F6
	// MOVF	F6, res+8(FP)
	// RET
	BGT	fatan22_pi2
	BLT	fatan22_neg_pi2
	MOVF	F6, res+8(FP)
	RET
fatan22_neg_pi2:
	MOVF	$-1.570796326794896557998981734272092580795288085938, F6
	MOVF	F6, res+8(FP)
	RET
fatan22_pi2:
	MOVF	$1.570796326794896557998981734272092580795288085938, F6
	MOVF	F6, res+8(FP)
	RET


TEXT ·Scalef32(SB),7,$0
	B ·scalef32(SB)
