//go:build !linux

package v0

import (
	"fmt"
	"os"
	"sync"
)

// New is the constructor for Writer.
func New(fpath string, options ...WriteOption) (*Writer, error) {
	f, err := os.Create(fpath)
	if err != nil {
		return nil, err
	}

	if err := f.Chmod(0600); err != nil {
		return nil, err
	}

	if _, err = f.Seek(reservedHeader, 0); err != nil {
		return nil, fmt.Errorf("was unable to seek %d bytes into the file: %q", reservedHeader, err)
	}

	w := &Writer{
		file:   f,
		offset: reservedHeader,
		index:  make(index, 0, 1000),
		mu:     sync.Mutex{},
	}
	for _, option := range options {
		option(w)
	}
	return w, nil
}
