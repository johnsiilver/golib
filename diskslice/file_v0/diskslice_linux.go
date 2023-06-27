//go:build linux

package v0

import (
	"bufio"
	"os"
	"sync"
	"syscall"

	"github.com/brk0v/directio"
)

// New returns a new Writer that writes to file "p".
func New(p string) (Writer, error) {
	f, err := os.OpenFile(p, os.O_CREATE+os.O_WRONLY+syscall.O_DIRECT, 0666)
	if err != nil {
		return nil, err
	}

	dio, err := directio.New(f)
	if err != nil {
		return nil, err
	}

	w := bufio.NewWriterSize(dio, 67108864)
	header := [64]byte{}
	_, err = w.Write(header[:])
	if err != nil {
		return nil, err
	}

	return &writer{
		name:   p,
		file:   f,
		dio:    dio,
		buf:    w,
		offset: reservedHeader,
		index:  make(index, 0, 1000),
		Mutex:  sync.Mutex{},
	}, nil
}
