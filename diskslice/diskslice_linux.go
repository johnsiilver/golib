//go:build linux

package diskslice

import (
	"bufio"
	"os"
	"sync"
	"syscall"

	"github.com/brk0v/directio"
)

// New is the constructor for Writer.
func New(fpath string, options ...WriteOption) (*Writer, error) {
	f, err := os.OpenFile(fpath, os.O_CREATE+os.O_WRONLY+syscall.O_DIRECT, 0666)
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

	wr := &Writer{
		name:   fpath,
		file:   f,
		buf:    w,
		dio:    dio,
		offset: reservedHeader,
		index:  make(index, 0, 1000),
		mu:     sync.Mutex{},
	}
	for _, option := range options {
		option(wr)
	}
	return wr, nil
}
