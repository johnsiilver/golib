package diskstack

import (
	"fmt"
	"io"
)

// reverse is a wrapper around an io.ReadWriter that will assume all data
// is in the reverse format. See methods for more information.
// reverse is not thread-safe!
type Reverse struct {
	RW io.ReadWriteSeeker
}

// Read implements io.Reader, but in a unique way.  Instead of reading the
// first byte from the stream into b[0], it will read it into b[len(b)-1],
// the second into b[len(b)-2], etc ...
// This implementation costs 2 Seeks + 1 Read + an O(n/2) op.
func (r Reverse) Read(b []byte) (n int, err error) {
	length := len(b)
	if length == 0 {
		return 0, nil
	}

	currentOffset, _ := r.RW.Seek(0, 1) // This should give us our current offset in the file.

	var eof error
	switch {
	case currentOffset == 0:
		return 0, io.EOF
		// The []byte is < the rest of the file, so only suck the entire []byte
		// in size.
	case currentOffset-int64(length) > 0:
		n = length
		// The []byte is >= the rest of the file, so only read in the length of the
		// file that is left.  Also, return io.EOF.
	default:
		n = int(currentOffset)
		eof = io.EOF
	}

	if _, err = r.RW.Seek(int64(-n), 1); err != nil {
		r.RW.Seek(currentOffset, 1) // Try to return to last position.
		return 0, fmt.Errorf("wtf error: either the data is changing while we are reading or Seek() is returning a bad offset: %s", err)
	}

	_, err = r.RW.Read(b)
	if err != nil {
		r.RW.Seek(currentOffset, 1) // Try to return to last position.
		return 0, fmt.Errorf("problem calling Read(): either the data is changing while we are reading or Seek() is returning a bad offset: %s", err)
	}

	r.reverseSlice(b[:n])

	if _, err = r.RW.Seek(int64(-n), 1); err != nil {
		return 0, fmt.Errorf("problem Seeking back: the data is changing underneath us: %s", err)
	}

	return n, eof
}

func (r Reverse) reverseSlice(sl []byte) {
	end := len(sl) - 1
	for front := 0; front < end; front++ {
		x := sl[front]
		sl[front] = sl[end]
		sl[end] = x
		end--
	}
}

// Write implements io.Writer, but in a unique way.  Instead or writing the
// first byte of the slice to the stream, it writes p[len(p)-1], then
// p[len(p)-2] and so on.
func (r Reverse) Write(p []byte) (n int, err error) {
	length := len(p)
	if length == 0 {
		return 0, nil
	}
	r.reverseSlice(p)
	return r.RW.Write(p)
}

func (r Reverse) Seek(offset int64, whence int) (int64, error) {
	return r.RW.Seek(offset, whence)
}
