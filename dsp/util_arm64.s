#include "textflag.h"

TEXT ·rotate90FilterAsm(SB), NOSPLIT, $0
    B ·rotate90Filter(SB)

TEXT ·i32Rotate90FilterAsm(SB), NOSPLIT, $0
    B ·i32Rotate90Filter(SB)
