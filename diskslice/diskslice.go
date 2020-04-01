/*
Package diskslice provides an array of entries on disk that can be used without reading the list into
memory. Like diskmap, this is a great quick and dirty way to serve data off disk from a single file
that can be managed, moved and trivially read. More complex use cases require more complex layouts
involving multiple files, lock files, etc.... This makes no attempt to provide that.

Read call without a cached index consist of:
	* A single seek to an index entry
	* Two 8 byte reads for data offset and len
	* A seek to the data
	* A single read of the value
Total: 2 seeks and 3 reads

Read call with cached index consits of:
	* A single read to the data

If doing a range over a large file or lots of range calls, it is optimal to have the Reader cache
the index.  Every 131,072 entries consumes 1 MiB of cached memory.

File format is as follows:
	<file>
	<reserve header space>
		[index offset]
		[number of entries]
	</reserve header space>
	<data section>
		[byte value]
		[byte value]
		...
	</data section>
	<index section>
		[data offset]
		[data length]
		...
	</index section>
	</file>
*/
package diskslice

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// reservedHeader is the size, in bytes, of the reserved header portion of the file.
const reservedHeader = 64

// endian is the endianess of all our binary encodings.
var endian = binary.LittleEndian

// table is a list of entries that are eventually encoded onto disk at the end of the file.
type index []entry

// entry is an entry in the index to allow locating data associated with a index.
type entry struct {
	// offset is the offset from the start of the file to locate the value associated with an index.
	offset int64

	// length is the length of the data from offset above.
	length int64
}

// value holds the data needed to locate a value of a key/value pair on disk.
type value struct {
	// offset is the offset from the start of the file to locate the value associated with an index.
	offset int64

	// length is the length of the data from offset above.
	length int64
}

// Writer provides methods for writing an array of values to disk that can be read without
// reading the file back into memory.
type Writer struct {
	file   *os.File
	index  index
	offset int64
	num    int64
	mu     sync.Mutex

	interceptor func(dst io.Writer) io.WriteCloser
}

// WriteOption is an option to the New() constructor.
type WriteOption func(w *Writer)

// WriteIntercept pipes the input via Write() through the passed io.WriteCloser.
// This is most often used to apply compression.
func WriteIntercept(interceptor func(dst io.Writer) io.WriteCloser) WriteOption {
	return func(w *Writer) {
		w.interceptor = interceptor
	}
}

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

// Write writes a byte slice to the diskslice.
func (w *Writer) Write(b []byte) error {
	if w.interceptor != nil {
		buff := bytes.Buffer{}
		writer := w.interceptor(&buff)
		if _, err := writer.Write(b); err != nil {
			return fmt.Errorf("the internal interceptor had a Write() error: %s", err)
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("the internal interceptor had a Close() error: %s", err)
		}
		b = buff.Bytes()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Write(b); err != nil {
		return fmt.Errorf("problem writing value to disk: %q", err)
	}

	w.index = append(
		w.index,
		entry{
			offset: w.offset,
			length: int64(len(b)),
		},
	)

	w.offset += int64(len(b))
	w.num++

	return nil
}

// Close closes the file for writing and writes our index to the file.
func (w *Writer) Close() error {
	// Write each data offset, then the length of the key, then finally the key to disk (our index) for each entry.
	for _, entry := range w.index {
		if err := binary.Write(w.file, endian, entry.offset); err != nil {
			return fmt.Errorf("could not write offset value %d: %q", entry.offset, err)
		}

		if err := binary.Write(w.file, endian, entry.length); err != nil {
			return fmt.Errorf("could not write data length: %q", err)
		}
	}

	// Now that we've written all our data to the end of the file, we can go back to our reserved header
	// and write our offset to the index at the beginnign of the file.
	if _, err := w.file.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek to beginning of the file: %q", err)
	}

	// Write the offset to the index to our reserved header.
	if err := binary.Write(w.file, endian, w.offset); err != nil {
		return fmt.Errorf("could not write the index offset to the reserved header: %q", err)
	}

	// Write the number of values.
	if err := binary.Write(w.file, endian, w.num); err != nil {
		return fmt.Errorf("could not write the number of values to the reserved header: %q", err)
	}

	if err := w.file.Sync(); err != nil {
		return err
	}

	return w.file.Close()
}

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
