// Modified from golang.org/x/sys/unix (BSD 3-Clause, see LICENSE).

//go:build dragonfly || freebsd || linux || netbsd

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
	r, _, e := syscall.Syscall(
		syscall.SYS_WRITEV,
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
