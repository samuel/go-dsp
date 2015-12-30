#include "textflag.h"

TEXT ·lowPassDownsampleComplexFilterAsm(SB), NOSPLIT, $0
	JMP ·lowPassDownsampleComplexFilter(SB)

TEXT ·lowPassDownsampleRationalFilterAsm(SB), NOSPLIT, $0
	JMP ·lowPassDownsampleRationalFilter(SB)
