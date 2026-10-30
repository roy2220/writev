package writev_test

import (
	"os"
	"syscall"
	"testing"

	"github.com/roy2220/stracetest"
	. "github.com/roy2220/writev"
	"github.com/stretchr/testify/require"
)

func init() {
	stracetest.DefaultStraceArgs = []string{"--quiet=all", "--seccomp-bpf"}
}

func TestWriter_WriteBuffersFastError(t *testing.T) {
	stracetest.Run(t, "writev error injected", func(t testing.TB, tempDir string) {
		f, err := os.Create(tempDir + "/test.txt")
		require.NoError(t, err)

		w, err := NewWriter(f)
		require.NoError(t, err)

		n, err := w.WriteBuffers([][]byte{[]byte("123"), []byte("abc")})
		require.ErrorIs(t, err, syscall.ENOSPC)
		require.Equal(t, 0, n)
	},
		"--trace-path={{TempDir}}/test.txt",
		"--trace=writev",
		"--inject=writev:error=ENOSPC",
	)

	stracetest.Run(t, "writev interrupted and retried", func(t testing.TB, tempDir string) {
		f, err := os.Create(tempDir + "/test.txt")
		require.NoError(t, err)

		w, err := NewWriter(f)
		require.NoError(t, err)

		n, err := w.WriteBuffers([][]byte{[]byte("123"), []byte("abc")})
		require.NoError(t, err)
		require.Equal(t, 6, n)
	},
		"--trace-path={{TempDir}}/test.txt",
		"--trace=writev",
		"--inject=writev:error=EINTR:when=1..3",
	)
}
