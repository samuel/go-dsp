#include "textflag.h"

TEXT ·fmDemodulateAsm(SB), NOSPLIT, $0
	JMP ·fmDemodulate(SB)
