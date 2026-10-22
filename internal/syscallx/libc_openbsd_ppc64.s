#include "textflag.h"

TEXT libc_writev_trampoline<>(SB),NOSPLIT,$0-0
	CALL	libc_writev(SB)
	RET
GLOBL	·libc_writev_trampoline_addr(SB), RODATA, $8
DATA	·libc_writev_trampoline_addr(SB)/8, $libc_writev_trampoline<>(SB)
