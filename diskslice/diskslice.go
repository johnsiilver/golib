package diskslice

/*
Package diskslice provides an array of entries on disk that can be used without reading the list into
memory. Like diskmap, this is a great quick and dirty way to serve data off disk from a single file
that can be managed, moved and trivially read. More complex use cases require more complex layouts
involving multiple files, lock files, etc.... This makes no attempt to provide that.

This is great when you want to write a vector to disk and never modify it afterwards.

On Linux this uses the O_DIRECT flag to bypass the OS's page cache for writing.

Read call without a cached index consist of:
  - A single seek to an index entry
  - An 8 byte reads for data offset
  - A seek to the data
  - A single read of the data length
  - A single read of the value

Total: 2 seeks and 3 reads

Read call with cached index consits of:
  - A single seek to the data offset
  - A single read to the data

If only doing ranges and not random reads, you should not bother to enable caching. There is little benefit
as only a single seek is required to find the starting data offset.

This package supports two versions of the data format. Version 0 is the original format and is significantly
slower than version 1 on Range calls (Version 1 is 7x faster). Version 1 is the default for writing and the
Reader will automatically detect the format.

File format is as follows:

	<file>
	<reserve header space(64 bytes)>
		[index offset(int64)]
		[number of entries(int64)]
		[version number(float32)]
	</reserve header space>
	<data section>
		[data length(int64)]
		[byte value(variable)]
		[data length(int64)]
		[byte value(variable)]
		...
	</data section>
	<index section>
		[data offset(int64)]
		...
	</index section>
	</file>
*/

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/johnsiilver/golib/diskslice/file_v0"

	"github.com/brk0v/directio"
)

const version float32 = 1.0

// reservedHeader is the size, IN BYTES (not BITS), of the reserved header portion of the file.
const reservedHeader = 64

// endian is the endianess of all our binary encodings.
var endian = binary.LittleEndian

// index is a list of data offsets that are encoded onto disk at the end of the file.
// Each index is the int64 offset from the start of the file to the start of the data.
type index []int64

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

// Writer provides methods for writing an array of values to disk that can be read without
// reading the file back into memory.
type Writer struct {
	name     string
	file     *os.File
	dio      *directio.DirectIO
	buffSize int
	buf      *bufio.Writer
	index    index
	header   header
	offset   int64
	mu       sync.Mutex

	useV0       bool
	v0          *file_v0.Writer
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

// WriteV0 indicates that you want to write a V0 file format compatible version.
func WriteV0() WriteOption {
	return func(w *Writer) {
		w.useV0 = true
	}
}

// WriteBuffer sets the size of the buffer used for writing to disk.
func WriteBuffer(sizeBytes int) WriteOption {
	return func(w *Writer) {
		w.buffSize = sizeBytes
	}
}

// Write writes a byte slice to the diskslice.
func (w *Writer) Write(b []byte) error {
	if w.v0 != nil {
		return w.v0.Write(b)
	}

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

	var writer io.Writer = w.file
	if w.buf != nil {
		writer = w.buf
	}
	if err := binary.Write(writer, endian, int64(len(b))); err != nil {
		return fmt.Errorf("could not write value length to disk: %q", err)
	}
	if _, err := writer.Write(b); err != nil {
		return fmt.Errorf("problem writing value to disk: %q", err)
	}

	w.index = append(w.index, w.offset)
	w.offset += 8 + int64(len(b)) // 8 bytes for the length of the value.
	w.header.num++

	return nil
}

// Close closes the file for writing and writes our index to the file.
func (w *Writer) Close() error {
	if w.v0 != nil {
		return w.v0.Close()
	}

	var writer io.Writer = w.file
	if w.buf != nil {
		writer = w.buf
	}

	w.header.indexOffset = w.offset

	// Write each data offset, then the length of the key, then finally the key to disk (our index) for each entry.
	for _, offset := range w.index {
		if err := binary.Write(writer, endian, offset); err != nil {
			return fmt.Errorf("could not write offset value %d: %q", offset, err)
		}
	}

	if w.buf != nil {
		w.buf.Flush()
		if w.dio != nil {
			w.dio.Flush()
		}

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

	if err := w.header.write(w.file); err != nil {
		return err
	}

	if err := w.file.Sync(); err != nil {
		return err
	}

	return w.file.Close()
}
