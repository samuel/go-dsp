#include "textflag.h"

TEXT ·rotate90FilterAsm(SB), NOSPLIT, $0
	JMP ·rotate90Filter(SB)

TEXT ·i32Rotate90FilterAsm(SB), NOSPLIT, $0
	JMP ·i32Rotate90Filter(SB)
