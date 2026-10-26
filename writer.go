// Package writev writes multiple buffers to a file or connection efficiently.
package writev

import (
	"io"
	"net"
	"os"
	"slices"
	"sync"
	"syscall"
	"unsafe"

	"github.com/roy2220/writev/internal/syscallx"
)

// SyscallWriter is an io.Writer whose underlying file descriptor can be
// accessed for system calls, as provided by syscall.Conn.
//
// Typical implementations are *os.File, *net.TCPConn, *net.UDPConn, *net.IPConn
// and *net.UnixConn.
type SyscallWriter interface {
	io.Writer
	syscall.Conn
}

var (
	_ SyscallWriter = (*os.File)(nil)
	_ SyscallWriter = (*net.TCPConn)(nil)
	_ SyscallWriter = (*net.UDPConn)(nil)
	_ SyscallWriter = (*net.IPConn)(nil)
	_ SyscallWriter = (*net.UnixConn)(nil)
)

// Writer batch-writes multiple buffers to a file or connection. A Writer must
// be created with NewWriter; the zero value is not usable.
type Writer struct {
	w       io.Writer
	rawConn syscall.RawConn
}

// NewWriter returns a Writer that writes to syscallWriter.
func NewWriter(syscallWriter SyscallWriter) (Writer, error) {
	var rawConn syscall.RawConn
	if syscallx.IsSupported {
		var err error
		rawConn, err = syscallWriter.SyscallConn()
		if err != nil {
			return Writer{}, err
		}
	}
	return Writer{
		w:       syscallWriter,
		rawConn: rawConn,
	}, nil
}

// WriteBuffers batch-writes the given buffers to the underlying writer in
// order, without modifying them. It returns the number of bytes written and the
// error that stopped the write early, if any.
//
// On Linux/Darwin/*BSD, buffers are written contiguously: concurrent writes to
// the same underlying writer within a process cannot interleave between the
// buffers.
//
// On other platforms, buffers may be split across multiple Write calls to the
// underlying writer. As a result, concurrent writes to the same underlying
// writer within a process may interleave between the buffers. Callers requiring
// contiguous writes on these platforms should serialize their calls (for
// example, with a mutex).
func (w Writer) WriteBuffers(buffers [][]byte) (int, error) {
	if syscallx.IsSupported {
		return w.writeBuffersFast(buffers)
	} else {
		return w.writeBuffersSlow(buffers, defaultMaxBatchSize)
	}
}

var writevOpPool = sync.Pool{New: func() any { return new(writevOp) }}

func (w Writer) writeBuffersFast(buffers [][]byte) (int, error) {
	if l := len(buffers); l <= 1 {
		if l == 0 {
			return 0, nil
		}
		if len(buffers[0]) == 0 {
			return 0, nil
		}
		return w.w.Write(buffers[0])
	}

	op := writevOpPool.Get().(*writevOp)
	op.Init(buffers)
	defer func() {
		op.Reset()
		writevOpPool.Put(op)
	}()

	if err := w.rawConn.Write(op.Execute); err != nil {
		return op.N, err
	}
	return op.N, op.Err
}

type writevOp struct {
	Execute func(fd uintptr) bool
	N       int
	Err     error

	iovecs          []syscallx.Iovec
	remainingIovecs []syscallx.Iovec
}

func (o *writevOp) Init(buffers [][]byte) {
	if o.Execute == nil {
		o.Execute = o.execute // escape to heap
	}
	o.iovecs = appendBytes(o.iovecs, buffers)
	o.remainingIovecs = o.iovecs
}

func (o *writevOp) Reset() {
	clear(o.iovecs)
	o.iovecs = o.iovecs[:0]
	o.remainingIovecs = nil
	o.N = 0
	o.Err = nil
}

func (o *writevOp) execute(fd uintptr) bool {
	for {
		l := len(o.remainingIovecs)
		if l == 0 {
			return true
		}
		someIovecs := o.remainingIovecs[:min(l, syscallx.IOV_MAX)]
		n, err := syscallx.Writev(int(fd), someIovecs)
		if err != nil {
			switch err {
			case syscallx.EINTR:
				continue
			case syscallx.EAGAIN:
				return false
			}
			o.Err = err
			return true
		}
		if n == 0 {
			o.Err = io.ErrShortWrite
			return true
		}
		o.N += n
		o.remainingIovecs = consumeBytes(o.remainingIovecs, n)
	}
}

func appendBytes(iovecs []syscallx.Iovec, buffers [][]byte) []syscallx.Iovec {
	for _, buffer := range buffers {
		l := len(buffer)
		if l == 0 {
			continue
		}
		base := unsafe.Pointer(unsafe.SliceData(buffer))
		iovecs = append(iovecs, makeIovec(base, l))
	}
	return iovecs
}

func consumeBytes(iovecs []syscallx.Iovec, n int) []syscallx.Iovec {
	i := 0
	for _, iovec := range iovecs {
		l := int(iovec.Len)
		if n < l {
			base := unsafe.Pointer(iovec.Base)
			iovecs[i] = makeIovec(unsafe.Add(base, n), l-n)
			break
		}
		n -= l
		i++
	}
	return iovecs[i:]
}

func makeIovec(base unsafe.Pointer, l int) syscallx.Iovec {
	var iovec syscallx.Iovec
	*(*unsafe.Pointer)(unsafe.Pointer(&iovec.Base)) = base
	iovec.SetLen(l)
	return iovec
}

const defaultMaxBatchSize = 64 * 1024

func (w Writer) writeBuffersSlow(buffers [][]byte, maxBatchSize int) (int, error) {
	var (
		batchOffset = 0
		batchSize   = 0
		n           = 0
	)
	flushBatch := func(nextBatchOffset int) error {
		var err error
		if batchSize >= 1 {
			batch := buffers[batchOffset:nextBatchOffset]
			var nn int
			nn, err = w.mergeAndWriteBuffers(batch, batchSize)
			n += nn
		}
		batchOffset = nextBatchOffset
		batchSize = 0
		return err
	}
	for i, buffer := range buffers {
		l := len(buffer)
		if l >= maxBatchSize {
			if err := flushBatch(i); err != nil {
				return n, err
			}
			nn, err := w.w.Write(buffer)
			n += nn
			if err != nil {
				return n, err
			}
			batchOffset = i + 1
			continue
		}
		if batchSize+l > maxBatchSize {
			if err := flushBatch(i); err != nil {
				return n, err
			}
		}
		batchSize += l
	}
	if err := flushBatch(len(buffers)); err != nil {
		return n, err
	}
	return n, nil
}

var mergedBufferPool = sync.Pool{New: func() any { return new([]byte) }}

func (w Writer) mergeAndWriteBuffers(buffers [][]byte, totalBufferSize int) (int, error) {
	for _, buffer := range buffers {
		l := len(buffer)
		if l == totalBufferSize {
			return w.w.Write(buffer)
		}
		if l >= 1 {
			break
		}
	}

	mergedBuffer := mergedBufferPool.Get().(*[]byte)
	defer mergedBufferPool.Put(mergedBuffer)

	*mergedBuffer = slices.Grow((*mergedBuffer)[:0], totalBufferSize)
	for _, buffer := range buffers {
		*mergedBuffer = append(*mergedBuffer, buffer...)
	}
	return w.w.Write(*mergedBuffer)
}
