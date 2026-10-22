#include "textflag.h"

TEXT libc_writev_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_writev(SB)
GLOBL	·libc_writev_trampoline_addr(SB), RODATA, $4
DATA	·libc_writev_trampoline_addr(SB)/4, $libc_writev_trampoline<>(SB)
