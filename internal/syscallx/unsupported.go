//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package syscallx

const IsSupported = false

const IOV_MAX = 0

type Iovec struct {
	Base *byte
	Len  int
}

func (Iovec) SetLen(int) {}

var (
	EINTR  error
	EAGAIN error
)

func Writev(fd int, iovecs []Iovec) (int, error) { panic("unsupported") }
