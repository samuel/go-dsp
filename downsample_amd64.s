#include "textflag.h"

TEXT ·boxcarDecimateAsm(SB), NOSPLIT, $0
	JMP ·boxcarDecimate(SB)

TEXT ·rationalBoxcarDecimateAsm(SB), NOSPLIT, $0
	JMP ·rationalBoxcarDecimate(SB)
