//go:build linux

package diskmap

import (
	"bufio"
	"os"
	"sync"
	"syscall"

	"github.com/brk0v/directio"
)

// New returns a new Writer that writes to file "p".
func New(p string, options ...WriterOption) (Writer, error) {
	wo := writerOptions{}
	wo = wo.defaults()
	var err error
	for _, o := range options {
		wo, err = o(wo)
		if err != nil {
			return nil, err
		}
	}

	f, err := os.OpenFile(p, os.O_CREATE+os.O_WRONLY+syscall.O_DIRECT, 0666)
	if err != nil {
		return nil, err
	}

	dio, err := directio.New(f)
	if err != nil {
		return nil, err
	}

	w := bufio.NewWriterSize(dio, wo.bufferSize)
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
		indexMap: map[string]struct{}{},
		Mutex:  sync.Mutex{},
	}, nil
}
