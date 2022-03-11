#include "textflag.h"

TEXT ·fmDemodulateAsm(SB), NOSPLIT, $0
    B ·fmDemodulate(SB)
