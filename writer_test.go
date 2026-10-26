package writev_test

import (
	"crypto/md5"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"sync"
	"syscall"
	"testing"

	. "github.com/roy2220/writev"
	"github.com/roy2220/writev/internal/syscallx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNotImplemented = errors.New("not implemented")

type mockSyscallWriter struct {
	Writer           io.Writer
	SyscallConnError error
	RawConn          syscall.RawConn
	FD               uintptr
}

var _ SyscallWriter = (*mockSyscallWriter)(nil)

func (w *mockSyscallWriter) Write(p []byte) (int, error) {
	if w.Writer != nil {
		return w.Writer.Write(p)
	}
	return 0, errNotImplemented
}

func (w *mockSyscallWriter) SyscallConn() (syscall.RawConn, error) {
	if w.SyscallConnError != nil {
		return nil, w.SyscallConnError
	}
	if w.RawConn != nil {
		return w.RawConn, nil
	}
	return &mockRawConn{WriteError: errNotImplemented}, nil
}

type mockWriter struct {
	T          *testing.T
	WriteFuncs []mockWriteFunc
	WriteCount int
}

type mockWriteFunc = func(t *testing.T, p []byte) (int, error)

var _ io.Writer = (*mockWriter)(nil)

func (w *mockWriter) Write(p []byte) (int, error) {
	i := w.WriteCount
	if i == len(w.WriteFuncs) {
		w.T.Fatal("unexpected write")
	}
	w.WriteCount++
	return w.WriteFuncs[i](w.T, p)
}

type mockRawConn struct {
	WriteError error
	FD         uintptr
}

var _ syscall.RawConn = (*mockRawConn)(nil)

func (c *mockRawConn) Control(f func(fd uintptr)) error          { panic("unreachable") }
func (c *mockRawConn) Read(f func(fd uintptr) (done bool)) error { panic("unreachable") }
func (c *mockRawConn) Write(f func(fd uintptr) (done bool)) error {
	if c.WriteError != nil {
		return c.WriteError
	}
	f(c.FD)
	return nil
}

func TestNewWriterFailed(t *testing.T) {
	_, err := NewWriter(&mockSyscallWriter{
		SyscallConnError: errors.New("syscall conn error"),
	})
	require.ErrorContains(t, err, "syscall conn error")
}

func TestWriter_WriteBuffersFastFailed(t *testing.T) {
	if !syscallx.IsSupported {
		t.SkipNow()
	}

	buffers := [][]byte{[]byte("123"), []byte("456"), []byte("789")}

	{
		w, err := NewWriter(&mockSyscallWriter{
			RawConn: &mockRawConn{
				WriteError: errors.New("raw conn write error"),
			},
		})
		require.NoError(t, err)
		_, err = w.WriteBuffersFast(buffers)
		require.ErrorContains(t, err, "raw conn write error")
	}

	{
		w, err := NewWriter(&mockSyscallWriter{
			RawConn: &mockRawConn{
				FD: ^uintptr(0),
			},
		})
		require.NoError(t, err)
		_, err = w.WriteBuffersFast(buffers)
		require.ErrorContains(t, err, "bad file descriptor")
	}
}

func TestWriter_WriteBuffersFastRandomized(t *testing.T) {
	if !syscallx.IsSupported {
		t.SkipNow()
	}

	const N = 10000

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { r.Close(); w.Close() })

	ww, err := NewWriter(w)
	require.NoError(t, err)

	var seed [32]byte
	for i := range seed {
		seed[i] = uint8(rand.IntN(256))
	}
	cc8 := rand.NewChaCha8(seed)

	buffers := [16][]byte{}
	for i := range buffers {
		buffers[i] = make([]byte, 256)
	}

	hash1 := md5.New()
	hash2 := md5.New()
	n1 := 0
	n2 := 0

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := io.Copy(hash2, r)
		assert.NoError(t, err)
	}()

	for range N {
		l := rand.IntN(1 + len(buffers))
		for i, b := range buffers[:l] {
			b = b[:rand.IntN(1+cap(b))]
			_, err := io.ReadFull(cc8, b)
			require.NoError(t, err)
			buffers[i] = b

			nn, err := hash1.Write(b)
			require.NoError(t, err)
			n1 += nn
		}

		nn, err := ww.WriteBuffersFast(buffers[:l])
		require.NoError(t, err)
		n2 += nn
	}

	err = w.Close()
	require.NoError(t, err)
	wg.Wait()

	require.Equal(t, n1, n2)
	require.Equal(t, hash1.Sum(nil), hash2.Sum(nil))
}

func TestWriter_WriteBuffersFastIovecLimit(t *testing.T) {
	if !syscallx.IsSupported {
		t.SkipNow()
	}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { r.Close(); w.Close() })

	ww, err := NewWriter(w)
	require.NoError(t, err)

	var seed [32]byte
	for i := range seed {
		seed[i] = uint8(rand.IntN(256))
	}
	cc8 := rand.NewChaCha8(seed)

	data := make([]byte, 1024)
	_, err = io.ReadFull(cc8, data)
	require.NoError(t, err)

	hash1 := md5.New()
	hash2 := md5.New()
	n1 := 0

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := io.Copy(hash2, r)
		assert.NoError(t, err)
	}()

	buffers := make([][]byte, 2048)
	for i := range buffers {
		b := data[:rand.IntN(1+len(data))]
		buffers[i] = b
		nn, err := hash1.Write(b)
		require.NoError(t, err)
		n1 += nn
	}

	n2, err := ww.WriteBuffersFast(buffers)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)
	wg.Wait()

	require.Equal(t, n1, n2)
	require.Equal(t, hash1.Sum(nil), hash2.Sum(nil))
}

func TestWriter_WriteBuffersSlow(t *testing.T) {
	errMock := errors.New("mock error")

	testCases := [...]struct {
		Desc string
		// input
		Buffers      [][]byte
		MaxBatchSize int
		// output
		WriteFuncs []mockWriteFunc
		N          int
		Error      string
	}{
		{
			Desc: "single empty buffer triggers no write",
			Buffers: [][]byte{
				{},
			},
			MaxBatchSize: 10,
			WriteFuncs:   []mockWriteFunc{},
		},
		{
			Desc: "empty and nil buffers trigger no write",
			Buffers: [][]byte{
				{},
				nil,
				{},
			},
			MaxBatchSize: 10,
			WriteFuncs:   []mockWriteFunc{},
		},
		{
			Desc: "single non-empty buffer is written as-is",
			Buffers: [][]byte{
				[]byte("1"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "1", string(p))
					return len(p), nil
				},
			},
			N: 1,
		},
		{
			Desc: "empty buffers are skipped and non-empty buffers merge into one write",
			Buffers: [][]byte{
				[]byte("1"),
				{},
				nil,
				[]byte("2"),
				{},
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "12", string(p))
					return len(p), nil
				},
			},
			N: 2,
		},
		{
			Desc: "buffer exactly at max batch size is written as-is without merging",
			Buffers: [][]byte{
				[]byte("0123456789"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "0123456789", string(p))
					return len(p), nil
				},
			},
			N: 10,
		},
		{
			Desc: "batch fills up to max size and buffer at max batch size is written separately",
			Buffers: [][]byte{
				[]byte("012345678"),
				[]byte("X"),
				[]byte("0123456789"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "012345678X", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "0123456789", string(p))
					return len(p), nil
				},
			},
			N: 20,
		},
		{
			Desc: "batch flushes early when next buffer would exceed max size",
			Buffers: [][]byte{
				[]byte("0123"),
				[]byte("4567"),
				[]byte("AABB"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "01234567", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "AABB", string(p))
					return len(p), nil
				},
			},
			N: 12,
		},
		{
			Desc: "batch accumulating to exactly max size is merged and remainder flushes separately",
			Buffers: [][]byte{
				[]byte("0123"),
				[]byte("4567"),
				[]byte("AA"),
				[]byte("BB"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "01234567AA", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "BB", string(p))
					return len(p), nil
				},
			},
			N: 12,
		},
		{
			Desc: "buffer larger than max batch size is written as-is without merging",
			Buffers: [][]byte{
				[]byte("0123456789AB"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "0123456789AB", string(p))
					return len(p), nil
				},
			},
			N: 12,
		},
		{
			Desc: "oversized buffer in the middle splits the writes",
			Buffers: [][]byte{
				[]byte("12"),
				[]byte("0123456789AB"),
				[]byte("34"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "12", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "0123456789AB", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "34", string(p))
					return len(p), nil
				},
			},
			N: 16,
		},
		{
			Desc: "oversized first buffer writes directly and following buffers form a new batch",
			Buffers: [][]byte{
				[]byte("0123456789AB"),
				[]byte("12"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "0123456789AB", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "12", string(p))
					return len(p), nil
				},
			},
			N: 14,
		},
		{
			Desc: "batch with empties and one non-empty buffer is written directly without merging",
			Buffers: [][]byte{
				nil,
				{},
				[]byte("1"),
				nil,
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "1", string(p))
					return len(p), nil
				},
			},
			N: 1,
		},
		{
			Desc: "error on first write is returned with zero count",
			Buffers: [][]byte{
				[]byte("12"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					return 0, errMock
				},
			},
			N:     0,
			Error: "mock error",
		},
		{
			Desc: "error during merged batch write keeps partial count",
			Buffers: [][]byte{
				[]byte("12"),
				[]byte("34"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "1234", string(p))
					return 2, errMock
				},
			},
			N:     2,
			Error: "mock error",
		},
		{
			Desc: "error during oversized buffer write keeps prior writes count",
			Buffers: [][]byte{
				[]byte("12"),
				[]byte("0123456789AB"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "12", string(p))
					return len(p), nil
				},
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "0123456789AB", string(p))
					return 5, errMock
				},
			},
			N:     7,
			Error: "mock error",
		},
		{
			Desc: "error during batch flush triggered by oversized buffer keeps partial count",
			Buffers: [][]byte{
				[]byte("12"),
				[]byte("34"),
				[]byte("0123456789AB"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "1234", string(p))
					return 2, errMock
				},
			},
			N:     2,
			Error: "mock error",
		},
		{
			Desc: "error during batch flush triggered by exceeding max size keeps partial count",
			Buffers: [][]byte{
				[]byte("0123"),
				[]byte("4567"),
				[]byte("AABB"),
			},
			MaxBatchSize: 10,
			WriteFuncs: []mockWriteFunc{
				func(t *testing.T, p []byte) (int, error) {
					assert.Equal(t, "01234567", string(p))
					return 5, errMock
				},
			},
			N:     5,
			Error: "mock error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Desc, func(t *testing.T) {
			w := &mockWriter{
				T:          t,
				WriteFuncs: tc.WriteFuncs,
			}
			ww, err := NewWriter(&mockSyscallWriter{
				Writer: w,
			})
			require.NoError(t, err)
			n, err := ww.WriteBuffersSlow(tc.Buffers, tc.MaxBatchSize)
			require.Equal(t, len(tc.WriteFuncs), w.WriteCount)
			if tc.Error == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.Error)
			}
			require.Equal(t, tc.N, n)
		})
	}
}

func TestWriter_WriteBuffersSlowRandomized(t *testing.T) {
	const N = 10000

	var seed [32]byte
	for i := range seed {
		seed[i] = uint8(rand.IntN(256))
	}
	cc8 := rand.NewChaCha8(seed)

	buffers := [6][]byte{}
	for i := range buffers {
		buffers[i] = make([]byte, 100)
	}

	hash1 := md5.New()
	hash2 := md5.New()
	n1 := 0
	n2 := 0

	w, err := NewWriter(&mockSyscallWriter{
		Writer: hash2,
	})
	require.NoError(t, err)

	for range N {
		l := rand.IntN(1 + len(buffers))
		for i, b := range buffers[:l] {
			b = b[:rand.IntN(1+cap(b))]
			_, err := io.ReadFull(cc8, b)
			require.NoError(t, err)
			buffers[i] = b

			nn, err := hash1.Write(b)
			require.NoError(t, err)
			n1 += nn
		}

		nn, err := w.WriteBuffersSlow(buffers[:l], 50)
		require.NoError(t, err)
		n2 += nn
	}

	require.Equal(t, n1, n2)
	require.Equal(t, hash1.Sum(nil), hash2.Sum(nil))
}

func BenchmarkWriter(b *testing.B) {
	if !syscallx.IsSupported {
		b.SkipNow()
	}

	cc8 := rand.NewChaCha8([32]byte{})
	buffers := [][]byte{
		make([]byte, 32*1024),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		make([]byte, 32*1024),
	}
	for i, buffer := range buffers {
		if buffer == nil {
			buffers[i] = make([]byte, 1+rand.IntN(1000))
		}
		_, err := io.ReadFull(cc8, buffers[i])
		require.NoError(b, err)
	}

	totalBufferSize := 0
	for _, p := range buffers {
		totalBufferSize += len(p)
	}

	run := func(b *testing.B, write func(*os.File, Writer) (int, error)) {
		f, err := os.CreateTemp(b.TempDir(), "writev")
		require.NoError(b, err)
		b.Cleanup(func() { f.Close() })
		require.NoError(b, f.Truncate(int64(totalBufferSize)))

		w, err := NewWriter(f)
		require.NoError(b, err)

		b.SetBytes(int64(totalBufferSize))
		b.ReportAllocs()

		for b.Loop() {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				b.Fatal(err)
			}
			n, err := write(f, w)
			if err != nil {
				b.Fatal(err)
			}
			if n != totalBufferSize {
				b.Fatalf("short write: %d/%d", n, totalBufferSize)
			}
		}
	}

	b.Run("Writes", func(b *testing.B) {
		run(b, func(f *os.File, w Writer) (int, error) {
			n := 0
			for _, p := range buffers {
				if len(p) == 0 {
					continue
				}
				nn, err := f.Write(p)
				n += nn
				if err != nil {
					return n, err
				}
			}
			return n, nil
		})
	})
	b.Run("WriteBuffersFast", func(b *testing.B) {
		run(b, func(f *os.File, w Writer) (int, error) {
			return w.WriteBuffersFast(buffers)
		})
	})
	b.Run("WriteBuffersSlow", func(b *testing.B) {
		run(b, func(f *os.File, w Writer) (int, error) {
			return w.WriteBuffersSlow(buffers, DefaultMaxBatchSize)
		})
	})
}
