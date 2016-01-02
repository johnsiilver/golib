/*
Package diskmap provides disk storage of key/value pairs.  The data is immutable once written.
In addition the diskmap utilizes mmap on reads to make the random access faster.

Usage is simplistic:

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
  * read the initial 8 bytes into a int64 to get the offset to the index
  * seek to the index offset
  * read the data storage offset
  * read the key length
  * read the key
  * build map of key to disk offset using the data above.
*/
package diskmap

import (
  "encoding/binary"
  "fmt"
  "os"
  "sync"

  log "github.com/golang/glog"
  "github.com/johnsiilver/golib/mmap"
  "golang.org/x/net/context"
)

// reservedHeader is the size, in bytes, of the reserved header portion of the file.
const reservedHeader = 64

// endian is the endianess of all our binary encodings.
var endian = binary.LittleEndian

// Reader provides read access to the the diskmap file.
type Reader interface {
  // Get fetches key "k" and returns the value. Errors when key not found. Thread-safe.
  Read(k []byte) ([]byte, error)

  // Range allows iteration over all the key/value pairs stored in the diskmap. If not interating
  // over all values, Cancel() or a timeout should be used on the Context to prevent a goroutine leak.
  Range(ctx context.Context) chan KeyValue

  // Close closes the diskmap file.
  Close() error
}

// Writer provides write access to the diskmap file.  An error on write makes the Writer unusable.
type Writer interface {
  // Write writes a key/value pair to disk.  Thread-safe.
  Write(k, v []byte) error

  // Close syncronizes the file to disk and closes it.
  Close() error
}

// KeyValue holds a key/value pair.
type KeyValue struct {
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

// writer implements Writer.
type writer struct {
  file *os.File
  index index
  offset int64
  num int64
  sync.Mutex
}

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
    file: f,
    offset: reservedHeader,
    index: make(index, 0, 1000),
    Mutex: sync.Mutex{},
  }, nil
}

// Write implements Writer.Write().
func (w *writer) Write(k, v []byte) error {
  w.Lock()
  defer w.Unlock()

  if _, err := w.file.Write(v); err != nil {
    return fmt.Errorf("problem writing key/value to disk: %q", err)
  }

  w.index = append(
    w.index,
    entry{
      key: k,
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
  // Write each data offset, then the length of the key, then finally the key to disk (our index) for each entry.
  for _, entry := range w.index {
    if err := binary.Write(w.file, endian, entry.offset); err != nil {
      return fmt.Errorf("could not write offset value %d: %q", entry.offset, err)
    }

    if err := binary.Write(w.file, endian, entry.length); err != nil {
      return fmt.Errorf("could not write data length: %q", err)
    }

    if err := binary.Write(w.file, endian, int64(len(entry.key))); err != nil {
      return fmt.Errorf("could not write key length: %q", err)
    }

    if _, err := w.file.Write(entry.key); err != nil {
      return fmt.Errorf("could not write key to disk: %q", err)
    }
  }

  // Now that we've written all our data to the end of the file, we can go back to our reserved 16 bytes
  // and write our offset to the index at the beginnign of the file.
  if _, err := w.file.Seek(0, 0); err != nil {
    return fmt.Errorf("could not seek to beginning of the file: %q", err)
  }

  // Write the offset to the index to our reserved header.
  if err := binary.Write(w.file, endian, w.offset); err != nil {
    return fmt.Errorf("could not write the index offset to the reserved header: %q", err)
  }

  // Write the number of key/value pairs.
  if err := binary.Write(w.file, endian, w.num); err != nil {
    return fmt.Errorf("could not write the number of key/value pairs to the reserved header: %q", err)
  }

  if err := w.file.Sync(); err != nil {
    return err
  }

  return w.file.Close()
}

// reader implements Reader.
type reader struct {
  // index is the key to offset data mapping.
  index map[string]value

  // file holds the mapping file in mmap.
  file mmap.Map
}

// Open returns a Reader for a file written by a Writer.
func Open(p string) (Reader, error) {
  f, err := os.Open(p)
  if err != nil {
    return nil, err
  }

  m, err := mmap.NewMap(f, mmap.Flag(mmap.Shared), mmap.Prot(mmap.Read))
  if err != nil {
    return nil, fmt.Errorf("problems mmapping %q: %q", p, err)
  }

  var (
    offset int64
    num int64
  )

  // Read our reserved header.
  if err := binary.Read(m, endian, &offset); err != nil {
    return nil, fmt.Errorf("cannot read index offset: %q", err)
  }

  if err := binary.Read(m, endian, &num); err != nil {
    return nil, fmt.Errorf("cannot read number of entries from reserved header: %q", err)
  }

  if _, err := m.Seek(offset, 0); err != nil {
    return nil, fmt.Errorf("cannot seek to index offset: %q", err)
  }

  kv := make(map[string]value, num)

  var dOff, dLen, kLen int64

  // Read the index data into a map.
  for i := int64(0); i < num; i++ {
    if err := binary.Read(m, endian, &dOff); err != nil {
      return nil, fmt.Errorf("cannot read a data offset in index: %q", err)
    }

    if err := binary.Read(m, endian, &dLen); err != nil {
      return nil, fmt.Errorf("cannot read a key offset in index: %q", err)
    }

    if err := binary.Read(m, endian, &kLen); err != nil {
      return nil, fmt.Errorf("cannot read a key offset in index: %q", err)
    }

    key := make([]byte, kLen)

    if _, err := m.Read(key); err != nil {
      return nil, fmt.Errorf("error reading in a key from the index: %q", err)
    }

    kv[string(key)] = value{
      offset: dOff,
      length: dLen,
    }
  }

  return reader{index: kv, file: m}, nil
}

// Read implements Reader.Read().
func (r reader) Read(k []byte) ([]byte, error) {
  if v, ok := r.index[string(k)]; ok {
    if _, err := r.file.Seek(v.offset, 0); err != nil {
      return nil, fmt.Errorf("cannot reach offset for key supplied by the index: %q", err)
    }
    b := make([]byte, v.length)
    if _, err := r.file.Read(b); err != nil {
      return nil, fmt.Errorf("error reading value from file: %q", err)
    }
    return b, nil
  }

  return nil, fmt.Errorf("key was not found")
}

// Range implements Reader.Range().
func (r reader) Range(ctx context.Context) chan KeyValue {
  ch := make(chan KeyValue, 10)

  go func() {
    defer close(ch)

    for k, _ := range r.index {
      v, err := r.Read([]byte(k))
      if err != nil {
        log.Errorf("key %q had no data associated with it (this is bad!): %q", k, err)
        continue
      }

      select {
      case ch <-KeyValue{[]byte(k), v}:
        // Do nothing.
      case <-ctx.Done():
        return
      }
    }
  }()

  return ch
}

// Close implements Reader.Close().
func (r reader) Close() error {
  return r.file.Close()
}
