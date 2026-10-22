# writev

[![Go Reference](https://pkg.go.dev/badge/github.com/roy2220/writev.svg)](https://pkg.go.dev/github.com/roy2220/writev)
[![Coverage](./.badges/coverage.svg)](#)

Package `writev` writes multiple buffers to a file or connection efficiently.

> [!NOTE]
> Except for the documentation, every line of code is human-crafted. **No AI slop included**.

## Why not `net.Buffers`?

1. `net.Buffers` batches writes only for `net.Conn`. This package also batches
   writes for `os.File`.

2. This package leaves the input buffers intact, whereas `net.Buffers.WriteTo`
   modifies them.

## Usage

### Example: Writev on `os.File`

```go
package main

import (
    "fmt"
    "io"
    "log"
    "os"

    "github.com/roy2220/writev"
)

func main() {
    f, err := os.CreateTemp("", "writev")
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        f.Close()
        os.Remove(f.Name())
    }()

    n, err := writev.Writev(f, [][]byte{
        []byte("hello, "),
        []byte("writev"),
        []byte("!"),
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(n)

    if _, err := f.Seek(0, io.SeekStart); err != nil {
        log.Fatal(err)
    }
    data, err := io.ReadAll(f)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(data))

    // Output:
    // 14
    // hello, writev!
}
```

## Limitations

* On Linux/Darwin/*BSD, the buffers are written in an atomic manner and other
  buffers from concurrent writes in the same process cannot interleave between
  them.

* On other platforms, the buffers may be split across multiple `Write` calls,
  so other buffers from concurrent writes in the same process may interleave
  between them. Callers that need an atomic write there should serialize their
  calls, for example with a mutex.

## License

MIT
