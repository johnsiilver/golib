//go:build !linux

package diskslice

import (
	"bufio"
	"os"
	"sync"

	"github.com/johnsiilver/golib/diskslice/file_v0"
)

// New is the constructor for Writer.
func New(fpath string, options ...WriteOption) (*Writer, error) {
	wr := &Writer{}
	for _, option := range options {
		option(wr)
	}
	if wr.useV0 {
		v0Options := make([]file_v0.WriteOption, 0, len(options))
		if wr.interceptor != nil {
			v0Options = append(v0Options, file_v0.WriteIntercept(wr.interceptor))
		}
		var err error
		wr.v0, err = file_v0.New(fpath, v0Options...)
		if err != nil {
			return nil, err
		}
		return wr, nil
	}

	f, err := os.Create(fpath)
	if err != nil {
		return nil, err
	}

	if err := f.Chmod(0600); err != nil {
		return nil, err
	}

	header := [reservedHeader]byte{}
	_, err = f.Write(header[:])
	if err != nil {
		return nil, err
	}

	wr = &Writer{
		file:     f,
		buffSize: 64 * 1024 * 1024,
		name:     fpath,
		index:    make(index, 0, 1000),
		offset:   reservedHeader,
		mu:       sync.Mutex{},
	}
	for _, option := range options {
		option(wr)
	}
	wr.buf = bufio.NewWriterSize(f, wr.buffSize)

	return wr, nil
}
