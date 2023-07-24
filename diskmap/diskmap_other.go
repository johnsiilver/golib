//go:build !linux

package diskmap

import (
	"bufio"
	"os"
	"sync"
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

	f, err := os.Create(p)
	if err != nil {
		return nil, err
	}

	if err := f.Chmod(0600); err != nil {
		return nil, err
	}

	w := bufio.NewWriterSize(f, wo.bufferSize)
	header := [64]byte{}
	_, err = w.Write(header[:])
	if err != nil {
		return nil, err
	}

	return &writer{
		file:   f,
		buf:    w,
		offset: reservedHeader,
		index:  make(index, 0, 1000),
		indexMap: map[string]struct{}{},
		Mutex:  sync.Mutex{},
	}, nil
}
