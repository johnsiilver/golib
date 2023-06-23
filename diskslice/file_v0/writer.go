package v0

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/brk0v/directio"
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

// Writer provides methods for writing an array of values to disk that can be read without
// reading the file back into memory.
type Writer struct {
	name   string
	file   *os.File
	dio    *directio.DirectIO
	buf    *bufio.Writer
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

	var writer io.Writer = w.file
	if w.buf != nil {
		writer = w.buf
	}
	if _, err := writer.Write(b); err != nil {
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
	var writer io.Writer = w.file
	if w.buf != nil {
		writer = w.buf
	}

	// Write each data offset, then the length of the key, then finally the key to disk (our index) for each entry.
	for _, entry := range w.index {
		if err := binary.Write(writer, endian, entry.offset); err != nil {
			return fmt.Errorf("could not write offset value %d: %q", entry.offset, err)
		}

		if err := binary.Write(writer, endian, entry.length); err != nil {
			return fmt.Errorf("could not write data length: %q", err)
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
