// Modified from golang.org/x/sys/unix (BSD 3-Clause, see LICENSE).

package syscallx

import (
	"syscall"
	"unsafe"
)

const IsSupported = true

const IOV_MAX = 1024

const (
	EINTR  = syscall.EINTR
	EAGAIN = syscall.EAGAIN
)

type Iovec = syscall.Iovec

func Writev(fd int, iovecs []Iovec) (int, error) {
	r, _, e := syscall_syscall(
		libc_writev_trampoline_addr,
		uintptr(fd),
		uintptr(unsafe.Pointer(&iovecs[0])),
		uintptr(len(iovecs)),
	)
	var err error
	if e != 0 {
		err = e
	}
	return int(r), err
}

//go:linkname syscall_syscall syscall.syscall
func syscall_syscall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)

var libc_writev_trampoline_addr uintptr

//go:cgo_import_dynamic libc_writev writev "libc.so"
