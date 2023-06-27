//go:build linux

package diskslice

import (
	"bufio"
	"os"
	"sync"
	"syscall"

	"github.com/johnsiilver/golib/diskslice/file_v0"

	"github.com/brk0v/directio"
)

// New is the constructor for Writer.
func New(fpath string, options ...WriteOption) (*Writer, error) {
	wr := &Writer{}
	for _, option := range options {
		option(wr)
	}
	if wr.useV0 {
		v0Options := make([]v0.WriteOption, 0, len(options))
		if wr.interceptor != nil {
			v0Options = append(v0Options, v0.WriteIntercept(wr.interceptor))
		}
		var err error
		wr.v0, err = v0.New(fpath, v0Options...)
		if err != nil {
			return nil, err
		}
		return wr, nil
	}

	f, err := os.OpenFile(fpath, os.O_CREATE+os.O_WRONLY+syscall.O_DIRECT, 0666)
	if err != nil {
		return nil, err
	}

	// Yeah, I know we are recreating wr and doing the options things again,
	// but this is cheap and easy.
	wr = &Writer{
		name:     fpath,
		file:     f,
		buffSize: 64 * 1024 * 1024,
		index:    make(index, 0, 1000),
		offset:   reservedHeader,
		mu:       sync.Mutex{},
	}
	for _, option := range options {
		option(wr)
	}

	dio, err := directio.New(f)
	if err != nil {
		return nil, err
	}

	w := bufio.NewWriterSize(dio, wr.buffSize)
	header := [reservedHeader]byte{}
	_, err = w.Write(header[:])
	if err != nil {
		return nil, err
	}
	wr.buf = w
	wr.dio = dio

	return wr, nil
}
