#include "funcdata.h"

TEXT ·Countdown(SB), $16-16
    NO_LOCAL_POINTERS
    MOVQ p0+0(FP), BX
block0:
    MOVQ $0, SI
    MOVQ SI, DI
block1:
    MOVQ BX, 0(SP)
    SUBQ $1, BX
    MOVQ DI, 8(SP)
    ADDQ 0(SP), DI
    MOVQ SI, AX
    CMPQ AX, 0(SP)
    JLT edge_yes1
    MOVQ 8(SP), DI
    JMP block2
edge_yes1:
    JMP block1
block2:
    MOVQ DI, r0+8(FP)
    RET
