package writev_test

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/roy2220/writev"
)

func ExampleWritev_file() {
	f, err := os.CreateTemp("", "writev")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	w, err := writev.NewWriter(f)
	if err != nil {
		log.Fatal(err)
	}

	n, err := w.WriteBuffers([][]byte{
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
