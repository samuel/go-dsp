#include "textflag.h"

TEXT ·fastAtan2Asm(SB), NOSPLIT, $0
	JMP ·fastAtan2(SB)

TEXT ·fastAtan2FineAsm(SB), NOSPLIT, $0
	JMP ·fastAtan2Fine(SB)

