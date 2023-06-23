package v0

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// Reader provides a reader for our diskslice.
type Reader struct {
	indexOffset int64
	len         int64
	mu          sync.Mutex

	interceptor func(src io.Reader) io.ReadCloser

	cacheIndex bool
	indexCache []int64
	lastSize   int64

	file *os.File
}

// ReadOption is an option to the Open() constructor.
type ReadOption func(r *Reader)

// ReadIntercept pipes the output for the reader through the passed io.ReadCloser.
// This is most often used to decompress data.
func ReadIntercept(interceptor func(src io.Reader) io.ReadCloser) ReadOption {
	return func(r *Reader) {
		r.interceptor = interceptor
	}
}

// CacheIndex provides an option to cache the data offsets index to spead up reads and ranges.
// Reduces our disk access at the cost of memory.
func CacheIndex() ReadOption {
	return func(r *Reader) {
		r.cacheIndex = true
	}
}

// Open is the constructor for Reader.
func Open(fpath string, options ...ReadOption) (*Reader, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}

	var (
		offset int64
		num    int64
	)

	// Read our reserved header.
	if err := binary.Read(f, endian, &offset); err != nil {
		return nil, fmt.Errorf("cannot read index offset: %q", err)
	}

	if err := binary.Read(f, endian, &num); err != nil {
		return nil, fmt.Errorf("cannot read number of entries from reserved header: %q", err)
	}

	r := &Reader{indexOffset: offset, len: num, file: f}
	for _, option := range options {
		option(r)
	}

	if r.cacheIndex {
		r.readOffsets()
	}
	return r, nil
}

// Len returns the number of entries in the array.
func (r *Reader) Len() int {
	return int(r.len)
}

// Read reads data at index i.
func (r *Reader) Read(i int) ([]byte, error) {
	const entryLen int64 = 16

	if i < 0 || i >= int(r.len) {
		return nil, fmt.Errorf("index out of bounds")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var b []byte

	// We have an index that is storing our offsets.
	if r.cacheIndex {
		dataOffset := r.indexCache[i]
		// Not the last entry
		if i+1 < r.Len() {
			b = make([]byte, r.indexCache[i+1]-dataOffset)
		} else { // the last entry
			b = make([]byte, r.lastSize)
		}
		if _, err := r.file.Seek(dataOffset, 0); err != nil {
			return nil, fmt.Errorf("cannot reach offset for index: %s", err)
		}
	} else { // We do not have an index that is storing our offsets.
		var dataOffset, dataLen int64

		offset := r.indexOffset + (int64(i) * entryLen)
		if _, err := r.file.Seek(offset, 0); err != nil {
			return nil, fmt.Errorf("cannot reach offset for key supplied by the index: %s", err)
		}
		if err := binary.Read(r.file, endian, &dataOffset); err != nil {
			return nil, fmt.Errorf("cannot read a data offset in index: %s", err)
		}

		if err := binary.Read(r.file, endian, &dataLen); err != nil {
			return nil, fmt.Errorf("cannot read a data len in index: %s", err)
		}

		if _, err := r.file.Seek(dataOffset, 0); err != nil {
			return nil, fmt.Errorf("cannot reach offset for index: %s", err)
		}

		b = make([]byte, dataLen)
	}

	if _, err := r.file.Read(b); err != nil {
		return nil, fmt.Errorf("error reading value from file: %s", err)
	}

	if r.interceptor != nil {
		buff := bytes.Buffer{}
		reader := r.interceptor(bytes.NewReader(b))
		if _, err := io.Copy(&buff, reader); err != nil {
			return nil, fmt.Errorf("the internal interceptor had a Read() error: %s", err)
		}
		if err := reader.Close(); err != nil {
			return nil, fmt.Errorf("the internal interceptor had a Close() error: %s", err)
		}
		b = buff.Bytes()
	}
	return b, nil
}

// readOffsets reads all the offsets from the file and stores them.
func (r *Reader) readOffsets() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.file.Seek(r.indexOffset, 0); err != nil {
		return fmt.Errorf("cannot reach offset for key supplied by the index: %q", err)
	}

	offsets := make([]int64, r.Len())

	var dataOffset int64
	for i := 0; i < r.Len(); i++ {
		if err := binary.Read(r.file, endian, &dataOffset); err != nil {
			return fmt.Errorf("cannot read a data offset in index: %s", err)
		}
		offsets[i] = dataOffset
		r.file.Seek(8, 1) // Skip the data size, we don't need it.
	}
	r.file.Seek(-8, 1)
	if err := binary.Read(r.file, endian, &r.lastSize); err != nil {
		return fmt.Errorf("cannot read final data length: %s", err)
	}

	r.indexCache = offsets
	return nil
}

// Value is a value returned by Range.
type Value struct {
	// Index is the index the value was at.
	Index int
	// Value was the value at the Index.
	Value []byte
	// Err indicates if we had an error reading from disk.
	Err error
}

// Range ranges over the disklist starting at start (inclusive) and ending before end (excluded).
// To range over all entries, Range(0, -1). The returned channel must be read until the channel is empty
// or a goroutine leak will ensue. If you need to cancel the Range(), cancel the context. You must
// check the context for an error if a deadline set or cancel is used on a Context in order to know
// if you received all values.
func (r *Reader) Range(ctx context.Context, start, end int) chan Value {
	ch := make(chan Value, 1)

	r.mu.Lock()
	defer r.mu.Unlock()

	if start < 0 {
		ch <- Value{Err: fmt.Errorf("Reader.Range() cannot have start value < 0")}
		close(ch)
		return ch
	}

	if end > r.Len() {
		ch <- Value{Err: fmt.Errorf("Reader.Rande() cannot have end value > the array length")}
		close(ch)
		return ch
	}

	if end < 0 {
		end = r.Len()
	}

	go func() {
		defer close(ch)
		for i := start; i < end; i++ {
			v, err := r.Read(i)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- Value{Index: i, Err: err}:
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case ch <- Value{Index: i, Value: v}:
			}
		}
	}()

	return ch
}
