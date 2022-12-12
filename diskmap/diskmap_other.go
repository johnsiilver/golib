//go:build !linux

package diskmap

import (
	"fmt"
	"os"
	"sync"
)

// New returns a new Writer that writes to file "p".
func New(p string) (Writer, error) {
	f, err := os.Create(p)
	if err != nil {
		return nil, err
	}

	if err := f.Chmod(0600); err != nil {
		return nil, err
	}

	if _, err = f.Seek(reservedHeader, 0); err != nil {
		return nil, fmt.Errorf("was unable to seek %d bytes into the file: %q", reservedHeader, err)
	}

	return &writer{
		file:   f,
		offset: reservedHeader,
		index:  make(index, 0, 1000),
		Mutex:  sync.Mutex{},
	}, nil
}
