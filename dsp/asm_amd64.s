#include "textflag.h"

// func hasSSE4() bool
// returns whether SSE4.1 is supported
TEXT ·hasSSE4(SB), NOSPLIT, $0
	XORQ AX, AX
	INCL AX
	CPUID
	SHRQ $19, CX
	ANDQ $1, CX
	MOVB CX, ret+0(FP)
	RET
