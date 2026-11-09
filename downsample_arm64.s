#include "textflag.h"

TEXT ·boxcarDecimateAsm(SB), NOSPLIT, $0
    B ·boxcarDecimate(SB)

TEXT ·rationalBoxcarDecimateAsm(SB), NOSPLIT, $0
    B ·rationalBoxcarDecimate(SB)
