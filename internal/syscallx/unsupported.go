// Modified from golang.org/x/sys/unix (BSD 3-Clause, see LICENSE).

//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package syscallx

const IsSupported = false

const IOV_MAX = 0

type Iovec struct {
	Base *byte
	Len  int
}

func (i *Iovec) SetLen(length int) {}

var (
	EINTR  error
	EAGAIN error
)

func Writev(fd int, iovecs []Iovec) (_ int, _ error) { return }
