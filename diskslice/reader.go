package diskslice

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/johnsiilver/golib/diskslice/file_v0"
)

// Reader provides a reader for our diskslice.
type Reader struct {
	header header
	mu     sync.Mutex

	interceptor func(src io.Reader) io.ReadCloser

	cacheIndex bool
	indexCache []int64

	file *os.File

	v0 *v0.Reader
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

	h := header{}
	if err := h.read(f); err != nil {
		return nil, err
	}

	r := &Reader{header: h, file: f}
	for _, option := range options {
		option(r)
	}

	if h.version == 0 {
		f.Close()

		voOpts := []v0.ReadOption{}
		if r.interceptor != nil {
			voOpts = append(voOpts, v0.ReadIntercept(r.interceptor))
		}
		if r.cacheIndex {
			voOpts = append(voOpts, v0.CacheIndex())
		}
		vr, err := v0.Open(fpath, voOpts...)
		if err != nil {
			return nil, err
		}
		r.v0 = vr
	}

	if r.cacheIndex {
		r.cacheOffsets()
	}
	return r, nil
}

// Len returns the number of entries in the array.
func (r *Reader) Len() int {
	return int(r.header.num)
}

const entryLen int64 = 8

var readerPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewReader([]byte{})
	},
}

// Read reads data at index i.
func (r *Reader) Read(i int) ([]byte, error) {
	if r.v0 != nil {
		return r.v0.Read(i)
	}

	if i < 0 || i >= int(r.header.num) {
		return nil, fmt.Errorf("index out of bounds")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var b []byte

	var dataOffset int64

	dataOffset, err := r.findOffset(i)
	if err != nil {
		return nil, err
	}

	if _, err := r.file.Seek(dataOffset, 0); err != nil {
		return nil, fmt.Errorf("cannot reach offset for index: %s", err)
	}

	// This gets the length of the data.
	var dataLen int64
	if err := binary.Read(r.file, endian, &dataLen); err != nil {
		return nil, fmt.Errorf("cannot read a data length for entry(%d): %s", i, err)
	}

	// This reads the data.
	b = make([]byte, dataLen)
	if _, err := r.file.Read(b); err != nil {
		return nil, fmt.Errorf("error reading value from file: %s", err)
	}

	return r.handleInterceptor(b)
}

func (r *Reader) handleInterceptor(b []byte) ([]byte, error) {
	if r.interceptor == nil {
		return b, nil
	}

	bytesReader := readerPool.Get().(*bytes.Reader)
	bytesReader.Reset(b)
	defer readerPool.Put(bytesReader)

	reader := r.interceptor(bytesReader)

	buff := bytes.Buffer{}
	if _, err := io.Copy(&buff, reader); err != nil {
		return nil, fmt.Errorf("the internal interceptor had a Read() error: %s", err)
	}

	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("the internal interceptor had a Close() error: %s", err)
	}
	return buff.Bytes(), nil
}

// cacheOffsets reads all the offsets from the file and stores them.
func (r *Reader) cacheOffsets() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.file.Seek(r.header.indexOffset, 0); err != nil {
		return fmt.Errorf("cannot reach offset for key supplied by the index: %q", err)
	}

	offsets := make([]int64, r.Len())

	var dataOffset int64
	for i := 0; i < r.Len(); i++ {
		if err := binary.Read(r.file, endian, &dataOffset); err != nil {
			return fmt.Errorf("cannot read a data offset in index: %s", err)
		}
		offsets[i] = dataOffset
	}

	r.indexCache = offsets
	return nil
}

// findOffset will find the offset the data at index i.
func (r *Reader) findOffset(i int) (dataOffset int64, err error) {
	if r.cacheIndex {
		return r.indexCache[i], nil
	}

	// We do not have an index that is storing our offsets.
	offset := r.header.indexOffset + (int64(i) * entryLen)
	if _, err := r.file.Seek(offset, 0); err != nil {
		return 0, fmt.Errorf("cannot reach offset for key supplied by the index: %s", err)
	}
	if err := binary.Read(r.file, endian, &dataOffset); err != nil {
		return 0, fmt.Errorf("cannot read a data offset in index: %s", err)
	}

	return dataOffset, nil
}

// Value is a value returned by Range.
type Value = v0.Value

type rangeOptions struct {
	buffSize int
}

func (o *rangeOptions) defaults() {
	o.buffSize = 64 * 1024 * 1024
}

type RangeOption func(o rangeOptions) rangeOptions

// WithReadBuffer sets the buffer size for the Range() channel. The default is 64 MiB.
func WithReadBuffer(buffSize int) RangeOption {
	return func(o rangeOptions) rangeOptions {
		o.buffSize = buffSize
		return o
	}
}

// Range ranges over the diskslice starting at start (inclusive) and ending before end (excluded).
// To range over all entries, Range(0, -1). The returned channel must be read until the channel is empty
// or a goroutine leak will ensue. If you need to cancel the Range(), cancel the context. You must
// check the context for an error if a deadline set or cancel is used on a Context in order to know
// if you received all values.
func (r *Reader) Range(ctx context.Context, start, end int, options ...RangeOption) chan Value {
	if r.v0 != nil {
		return r.v0.Range(ctx, start, end)
	}
	opts := rangeOptions{}
	for _, o := range options {
		opts = o(opts)
	}

	ch := make(chan Value, 1)

	r.mu.Lock()
	defer r.mu.Unlock()

	if start < 0 {
		ch <- Value{Err: fmt.Errorf("Reader.Range() cannot have start value < 0")}
		close(ch)
		return ch
	}

	if end > r.Len() {
		ch <- Value{Err: fmt.Errorf("Reader.Range() cannot have end value > the array length")}
		close(ch)
		return ch
	}

	if end < 0 {
		end = r.Len()
	}

	initialOffset, err := r.findOffset(start)
	if err != nil {
		ch <- Value{Err: fmt.Errorf("Reader.Range() could not find offset for starting value: %w", err)}
		close(ch)
		return ch
	}

	if _, err := r.file.Seek(initialOffset, 0); err != nil {
		ch <- Value{Err: fmt.Errorf("Reader.Range() could not seek to start of data: %w", err)}
		close(ch)
		return ch
	}

	var buff = bufio.NewReaderSize(r.file, opts.buffSize)

	go func() {
		defer close(ch)
		for i := start; i < end; i++ {
			v, err := r.getNextEntry(buff, i)
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

// getEntryHeader gets the next entries header, assuming we are at the end of an entry. This can be used in our "for"
// loop to get faster reads.
func (r *Reader) getNextEntry(f io.Reader, i int) ([]byte, error) {
	var dataLen int64

	if err := binary.Read(f, endian, &dataLen); err != nil {
		return nil, fmt.Errorf("cannot read a data length for entry(%d): %s", i, err)
	}

	b := make([]byte, dataLen)
	totalRead := 0
	for totalRead < int(dataLen) {
		n, err := f.Read(b[totalRead:])
		if err != nil {
			return nil, fmt.Errorf("truncated entry(%d), got %d bytes, expected %d", i, n, totalRead)
		}
		totalRead += n
	}

	return r.handleInterceptor(b)
}
