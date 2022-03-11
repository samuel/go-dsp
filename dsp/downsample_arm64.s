#include "textflag.h"

TEXT ·lowPassDownsampleComplexFilterAsm(SB), NOSPLIT, $0
    B ·lowPassDownsampleComplexFilter(SB)

TEXT ·lowPassDownsampleRationalFilterAsm(SB), NOSPLIT, $0
    B ·lowPassDownsampleRationalFilter(SB)
