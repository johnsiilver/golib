/*
Package diskmap provides disk storage of key/value pairs.  The data is immutable once written.
In addition the diskmap utilizes mmap on reads to make the random access faster.
On Linux, diskmap uses directio to speed up writes.

Unlike a regular map, keys can have duplicates. If you need this filtered out you
must do it before adding to the diskmap.

Usage is simplistic:

	// Create a new diskmap.
	p := path.Join(os.TempDir(), nextSuffix())
	w, err := diskmap.New(p)
	if err != nil {
	  panic(err)
	}

	// Write a key/value to the diskmap.
	if err := w.Write([]byte("hello"), []byte("world")); err != nil {
	  panic(err)
	}

	// Close the file to writing.
	w.Close()

	// Open the file for reading.
	m, err := diskmap.Open(p)
	if err != nil {
	  panic(err)
	}

	// Read the value at key "hello".
	v, err := m.Read([]byte("hello"))
	if err != nil {
	  panic(err)
	}

	// Print the value at key "hello".
	fmt.Println(string(v))

	// Loop through all entries in the map.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Make sure if we end the "range" early we don't leave any leaky goroutines.

	for kv := range m.Range(ctx) {
	  fmt.Printf("key: %s, value: %s", string(kv.Key), string(kv.Value))
	}

Storage details:

The disk storage is fairly straight forward.  Once all values have been written to disk, an index of the keys
and offsets is written to disk.  64 bytes are reserved at the start of the file to hold a int64 that gives
the offset where the index starts and the number of key/value pairs stored.  The additional space is reserved
for future use. All numbers are int64 values.

The file structure looks as follows:

	<file>
	  <reserve header space>
	    [index offset]
	    [number of key/value pairs]
	  </reserve header space>
	  <data section>
	    [byte value]
	    [byte value]
	    ...
	  </data section>
	  <index section>
	    [data offset]
	    [data length]
	    [key length]
	    [key]
	    ...
	  </index section>
	</file>

Reading the file is simply:
  - read the initial 8 bytes into a int64 to get the offset to the index
  - seek to the index offset
  - read the data storage offset
  - read the key length
  - read the key
  - build map of key to disk offset using the data above.
*/
package diskmap

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"unsafe"

	"github.com/brk0v/directio"
	"github.com/johnsiilver/golib/diskmap/internal/ordered"
)

// reservedHeader is the size, IN BYTES, of the reserved header portion of the file.
const reservedHeader = 64

const version float32 = 0.0

// endian is the endianess of all our binary encodings.
var endian = binary.LittleEndian

// ErrKeyNotFound indicates that a searched for key was not found.
var ErrKeyNotFound = fmt.Errorf("key was not found")

// Reader provides read access to the the diskmap file. If you fake this, you need
// to embed it in your fake.
type Reader interface {
	// Exists returns true if the key exists in the diskmap. Thread-safe.
	Exists(key []byte) bool

	// Keys returns all the keys in the diskmap. This reads only from memory
	// and does not access disk. Thread-safe.
	Keys(ctx context.Context) chan []byte

	// Read fetches key "k" and returns the value. If there are multi-key matches,
	// it returns the last key added. Errors when key not found. Thread-safe.
	Read(k []byte) ([]byte, error)

	// ReadAll fetches all matches to key "k". Does not error if not found. Thread-safe.
	ReadAll(k []byte) ([][]byte, error)

	// Range allows iteration over all the key/value pairs stored in the diskmap. If not interating
	// over all values, Cancel() or a timeout should be used on the Context to prevent a goroutine leak.
	Range(ctx context.Context) chan KeyValue

	// Close closes the diskmap file.
	Close() error
}

// Writer provides write access to the diskmap file.  An error on write makes the Writer unusable.
// If you fake this, you need to embed it in your fake.
type Writer interface {
	// Write writes a key/value pair to disk.  Thread-safe.
	Write(k, v []byte) error

	// Close syncronizes the file to disk and closes it.
	Close() error
}

// KeyValue holds a key/value pair.
type KeyValue struct {
	// Err indicates that there was an error in the return stream.
	Err error

	// Key is the key the value was stored at.
	Key []byte

	// Value is the value stored at Key.
	Value []byte
}

// table is a list of entries that are eventually encoded onto disk at the end of the file.
type index []entry

// entry is an entry in the index to allow locating data associated with a key.
type entry struct {
	// offset is the offset from the start of the file to locate the value associated with key.
	offset int64

	// length is the length of the data from offset above.
	length int64

	// key is the key part of the key/value pair.
	key []byte
}

// value holds the data needed to locate a value of a key/value pair on disk.
type value struct {
	// offset is the offset from the start of the file to locate the value associated with key.
	offset int64

	// length is the length of the data from offset above.
	length int64
}

type header struct {
	// indexOffset is the offset from the start of the file to the start of the index.
	indexOffset int64
	// num is the number of entries in the diskslice.
	num int64
	// version is the version of the diskslice data format.
	version float32 // only set when reading, not used in writing
}

func (h *header) read(f io.Reader) error {
	if err := binary.Read(f, endian, &h.indexOffset); err != nil {
		return fmt.Errorf("cannot read index offset: %q", err)
	}

	if err := binary.Read(f, endian, &h.num); err != nil {
		return fmt.Errorf("cannot read number of entries from reserved header: %q", err)
	}

	if err := binary.Read(f, endian, &h.version); err != nil {
		return fmt.Errorf("cannot read version from reserved header: %q", err)
	}
	return nil
}

func (h *header) write(f io.Writer) error {
	// Write the offset to the index to our reserved header.
	if err := binary.Write(f, endian, h.indexOffset); err != nil {
		return fmt.Errorf("could not write the index offset to the reserved header: %q", err)
	}

	// Write the number of values.
	if err := binary.Write(f, endian, h.num); err != nil {
		return fmt.Errorf("could not write the number of values to the reserved header: %q", err)
	}

	if err := binary.Write(f, endian, version); err != nil {
		return fmt.Errorf("could not write the version to the reserved header: %q", err)
	}
	return nil
}

// writer implements Writer.
type writer struct {
	header header
	name   string
	file   *os.File
	dio    *directio.DirectIO
	buf    *bufio.Writer
	index  index
	offset int64
	num    int64
	sync.Mutex
}

// Write implements Writer.Write().
func (w *writer) Write(k, v []byte) error {
	w.Lock()
	defer w.Unlock()

	var writer io.Writer
	if w.buf != nil {
		writer = w.buf
	} else {
		writer = w.file
	}

	if _, err := writer.Write(v); err != nil {
		return fmt.Errorf("problem writing key/value to disk: %q", err)
	}

	w.index = append(
		w.index,
		entry{
			key:    k,
			offset: w.offset,
			length: int64(len(v)),
		},
	)

	w.offset += int64(len(v))
	w.num += 1

	return nil
}

// Close implements Writer.Close().
func (w *writer) Close() error {
	var writer io.Writer
	if w.buf != nil {
		writer = w.buf
	} else {
		writer = w.file
	}

	// Write each data offset, then the length of the key, then finally the key to disk (our index) for each entry.
	for _, entry := range w.index {
		if err := binary.Write(writer, endian, entry.offset); err != nil {
			return fmt.Errorf("could not write offset value %d: %q", entry.offset, err)
		}

		if err := binary.Write(writer, endian, entry.length); err != nil {
			return fmt.Errorf("could not write data length: %q", err)
		}

		if err := binary.Write(writer, endian, int64(len(entry.key))); err != nil {
			return fmt.Errorf("could not write key length: %q", err)
		}

		if _, err := writer.Write(entry.key); err != nil {
			return fmt.Errorf("could not write key to disk: %q", err)
		}
	}
	if w.buf != nil {
		w.buf.Flush()
		w.dio.Flush()

		w.file.Close()
		f, err := os.OpenFile(w.name, os.O_RDWR, 0666)
		if err != nil {
			return err
		}
		w.file = f
	}

	// Now that we've written all our data to the end of the file, we can go back to our reserved header
	// and write our offset to the index at the beginnign of the file.
	if _, err := w.file.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek to beginning of the file: %q", err)
	}

	w.header.indexOffset = w.offset
	w.header.num = w.num
	w.header.version = version

	if err := w.header.write(w.file); err != nil {
		return fmt.Errorf("could not write reserved header: %q", err)
	}

	if err := w.file.Sync(); err != nil {
		return err
	}

	return w.file.Close()
}

// reader implements Reader.
type reader struct {
	header header

	// index is the key to offset data mapping.
	index *ordered.Map[string, []value]

	numReaders int
	readers    chan *os.File
}

type OpenOption func(r *reader) error

// WithNumReaders sets the number of readers to use when reading from the file.
// By default, 10 readers are used. Range() concurrently uses all available readers.
func WithNumReaders(n int) OpenOption {
	return func(r *reader) error {
		if n <= 0 {
			return fmt.Errorf("number of readers must be greater than 0")
		}
		r.numReaders = n
		return nil
	}
}

// Open returns a Reader for a file written by a Writer.
func Open(p string, options ...OpenOption) (Reader, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}

	h := header{}
	if err := h.read(f); err != nil {
		return nil, err
	}

	if _, err := f.Seek(h.indexOffset, 0); err != nil {
		return nil, fmt.Errorf("cannot seek to index offset: %q", err)
	}

	kv := ordered.New[string, []value]()
	r := &reader{index: kv, header: h, numReaders: 10}
	for _, o := range options {
		if err := o(r); err != nil {
			f.Close()
			return nil, err
		}
	}
	readers := make(chan *os.File, r.numReaders)
	r.readers = readers
	r.readers <- f
	for i := 0; i < r.numReaders-1; i++ {
		f, err := os.Open(p)
		if err != nil {
			for {
				select {
				case f := <-r.readers:
					f.Close()
				default:
					return nil, err
				}
			}
		}
		r.readers <- f
	}

	if err := r.doCache(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *reader) doCache() error {
	var dOff, dLen, kLen int64

	f := <-r.readers
	defer func() {
		r.readers <- f
	}()

	read := bufio.NewReaderSize(f, 64*1024*1024)

	// Read the index data into a map.
	for i := int64(0); i < r.header.num; i++ {
		if err := binary.Read(read, endian, &dOff); err != nil {
			return fmt.Errorf("cannot read a data offset in index: %q", err)
		}

		if err := binary.Read(read, endian, &dLen); err != nil {
			return fmt.Errorf("cannot read a key offset in index: %q", err)
		}

		if err := binary.Read(read, endian, &kLen); err != nil {
			return fmt.Errorf("cannot read a key offset in index: %q", err)
		}

		key := make([]byte, kLen)
		_, err := io.ReadFull(read, key)
		if err != nil {
			return fmt.Errorf("error reading in a key from the index: %q", err)
		}

		strKey := byteSlice2String(key)
		if sl, ok := r.index.Get(strKey); ok {
			sl = append(sl, value{offset: dOff, length: dLen})
			r.index.Set(strKey, sl)
			continue
		}

		r.index.Set(string(key), []value{{offset: dOff, length: dLen}})
	}
	return nil
}

// Exists returns true if the key exists in the index. There may be multiple entries.
// This prevents a disk read for key lookups vs using a Read() and checking for ErrKeyNotFound.
func (r *reader) Exists(key []byte) bool {
	k := byteSlice2String(key)
	_, ok := r.index.Get(k)
	return ok
}

// Keys returns all the keys in the map. There are no disk reads for this operation.
// Changing the returned []byte will change the underlying data in the index, don't do it.
func (r *reader) Keys(ctx context.Context) chan []byte {
	ch := make(chan []byte, 1)
	go func() {
		defer close(ch)
		for kv := range r.index.Range(ctx) {
			kk := UnsafeGetBytes(kv.Key)
			select {
			case <-ctx.Done():
				return
			case ch <- kk:

			}
		}
	}()
	return ch
}

// Read implements Reader.Read().
func (r *reader) Read(k []byte) ([]byte, error) {
	return r.read(k)
}

func (r *reader) read(k []byte) ([]byte, error) {
	vals, ok := r.index.Get(byteSlice2String(k))
	if !ok {
		return nil, ErrKeyNotFound
	}
	// If there are multiple values with the same key, only return the last one.
	v := vals[len(vals)-1]

	f := <-r.readers
	defer func() {
		r.readers <- f
	}()

	b := make([]byte, v.length)
	f.Seek(v.offset, 0)
	_, err := f.Read(b)
	if err != nil {
		return nil, fmt.Errorf("error reading value from file: %q", err)
	}

	return b, nil
}

// ReadAll implements Reader.ReadAll().
func (r *reader) ReadAll(k []byte) ([][]byte, error) {
	vals, ok := r.index.Get(byteSlice2String(k))
	if !ok {
		return nil, nil
	}

	f := <-r.readers
	defer func() {
		r.readers <- f
	}()

	sl := make([][]byte, 0, len(vals))
	for _, v := range vals {
		b := make([]byte, v.length)
		f.Seek(v.offset, 0)
		_, err := f.Read(b)
		if err != nil {
			return nil, fmt.Errorf("error reading value from file: %q", err)
		}
		sl = append(sl, b)
	}
	return sl, nil
}

// Range implements Reader.Range(). This does not guarantee order.
func (r *reader) Range(ctx context.Context) chan KeyValue {
	ch := make(chan KeyValue, r.numReaders)
	go func() {
		defer close(ch)

		var read *os.File
		select {
		case read = <-r.readers:
		case <-ctx.Done():
			ch <- KeyValue{Err: ctx.Err()}
			return
		}
		_, start := r.index.Oldest()
		_, err := read.Seek(start[0].offset, 0)
		if err != nil {
			ch <- KeyValue{Err: fmt.Errorf("error seeking to start of data: %q", err)}
			return
		}

		buff := bufio.NewReaderSize(read, 64*1024*1024)

		for kv := range r.index.Range(ctx) {
			for _, v := range kv.Value {
				b := make([]byte, v.length)
				_, err := io.ReadFull(buff, b)
				if err != nil {
					ch <- KeyValue{Err: fmt.Errorf("error reading value from file: %q", err)}
					return
				}
				select {
				case ch <- KeyValue{Key: UnsafeGetBytes(kv.Key), Value: b}:
				case <-ctx.Done():
					ch <- KeyValue{Err: ctx.Err()}
					return
				}
			}
		}
	}()

	return ch
}

// Close implements Reader.Close().
func (r *reader) Close() error {
	defer close(r.readers)
	for {
		select {
		case f := <-r.readers:
			f.Close()
		default:
			return nil
		}
	}
}

// UnsafeGetBytes retrieves the underlying []byte held in string "s" without doing
// a copy. Do not modify the []byte or suffer the consequences.
func UnsafeGetBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return (*[0x7fff0000]byte)(unsafe.Pointer(
		(*reflect.StringHeader)(unsafe.Pointer(&s)).Data),
	)[:len(s):len(s)]
}

func byteSlice2String(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}
