/*
Package diskstack implements a LIFO queue (known as a stack) on local disk.
The stack resizes a single file automatically according to the amount of
entries required.

Usage:
  s, err := New("/path/to/file", int(0))
  if err != nil {
    // Do something.
  }

  for i := 0; i < 10; i++ {
    if err := s.Push(i); err != nil {
      // Do something.
    }
  }

  var n int
  ok, err := s.Pop(&n)
  if err != nil {
    // Do something
  }
  if !ok {
    // Would mean the queue is empty, its not.
  }

  fmt.Println(n) // Prints 9

  s.Pop(&n)
  fmt.Println(n) // Prints 8

  fmt.Println("stack length: ", s.Len()) // Prints "stack length: 8"

  fmt.Println("stack size in bytes: ", s.Size())
*/
package diskstack

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

const int64Size = 8

// internalVersion tracks the current version of the disk representation that
// we read/write. We should be able to read back versions that are <= our
// version number.
const internalVersion = 1

// VersionInfo is used to encode version information for the disk stack into
// our files.  This struct can never have a field removed only added.
type VersionInfo struct {
	// Version is the version of encoding used to by stack to encode this.
	// This is not the same as the Semantic Version of the code.
	Version uint64
	// Created is when the file was actually created.
	Created int64
}

// Encode encodes the VersionInfo to the start of a file. If an error returns
// the file will be truncated to 0.
func (v VersionInfo) encode(f *os.File) (n int, err error) {
	defer func() {
		if err != nil {
			f.Truncate(0)
			f.Seek(0, 0)
		}
		f.Sync()
	}()

	if _, err = f.Seek(0, 0); err != nil {
		return 0, fmt.Errorf("could not seek the beginning of the file: %s", err)
	}

	buff := bytes.NewBuffer([]byte{})
	enc := gob.NewEncoder(buff)

	// Write our data to the buffer.
	if err = enc.Encode(v); err != nil {
		return 0, fmt.Errorf("could not gob encode the data: %s", err)
	}

	// Write our version number to disk.
	b := make([]byte, int64Size)
	binary.LittleEndian.PutUint64(b, v.Version)
	if _, err = f.Write(b); err != nil {
		return 0, fmt.Errorf("unable to write the version number to disk")
	}

	f.Seek(-int64Size, 2)
	if _, err := f.Read(b); err != nil {
		panic(err)
	}

	// Write our size header to disk.
	l := buff.Len()
	binary.LittleEndian.PutUint64(b, uint64(l))
	if _, err = f.Write(b); err != nil {
		return 0, fmt.Errorf("unable to write data size to disk: %s", err)
	}

	f.Seek(-int64Size, 2)
	if _, err := f.Read(b); err != nil {
		panic(err)
	}

	// Write the buffer to disk.
	if _, err = f.Write(buff.Bytes()); err != nil {
		return 0, fmt.Errorf("unable to write our version info to disk: %s", err)
	}

	return int(int64Size + int64Size + l), nil
}

func (v *VersionInfo) decode(f *os.File) error {
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek the beginning of the file: %s", err)
	}

	// Read version number back.
	b := make([]byte, int64Size)
	if _, err := f.Read(b); err != nil {
		return fmt.Errorf("cannot read the version number from the file: %s", err)
	}
	ver := binary.LittleEndian.Uint64(b)
	if ver > internalVersion {
		return fmt.Errorf("cannot read this file: current version number %d is greater than the libraries version number %d", ver, internalVersion)
	}

	// Read version block size.
	if _, err := f.Read(b); err != nil {
		return fmt.Errorf("cannot read the version block size back from the file: %s", err)
	}
	blockSize := binary.LittleEndian.Uint64(b)

	b = make([]byte, blockSize)
	if _, err := f.Read(b); err != nil {
		return fmt.Errorf("cannot read the version block from the file: %s, err")
	}

	dec := gob.NewDecoder(bytes.NewBuffer(b))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("cannot convert the version block on disk into a VersionInfo struct: %s", err)
	}
	return nil
}

// Stack implements a LIFO queue that can store a single object type that can
// be encoded by the gob encoder (encoding/gob) on local disk.
// The file is constantly resized based for each pop() and push() operation.
// All operations are thread-safe.
type Stack struct {
	storedType string
	f          *os.File
	rws        io.ReadWriteSeeker
	flush      bool

	mu     sync.Mutex // Protects everything below.
	length int64      // Only access with atomic operations.
	size   int64      // Dito.
}

// Option provides an optional argument to New().
type Option func(s *Stack)

// NoFlush indicates to not flush every write to disk.  This increases speed but
// at the cost of possibly losing a Push() or Pop().
func NoFlush() Option {
	return func(s *Stack) {
		s.flush = false
	}
}

// New creates a new instance of Stack.  p is the location of where to write the
// stack.  The file must not exist.  dataType is the value that you will store
// in the stack.  It must be gob encodable.  Also see Pop() for more information
// on what can go here.
func New(p string, dataType interface{}, options ...Option) (*Stack, error) {
	f, err := os.OpenFile(p, os.O_RDWR+os.O_CREATE+os.O_EXCL, 0666)
	if err != nil {
		return nil, err
	}

	vi := VersionInfo{Version: internalVersion, Created: time.Now().Unix()}
	viLen, err := vi.encode(f)
	if err != nil {
		return nil, err
	}

	if _, err := f.Seek(int64(viLen), 0); err != nil {
		return nil, fmt.Errorf("cannot seek past the VersionInfo header we wrote")
	}

	s := &Stack{flush: true, f: f, rws: f, length: 0, size: int64(viLen), storedType: reflect.TypeOf(dataType).Name()}

	for _, o := range options {
		o(s)
	}

	return s, nil
}

// Close closes the file backing the stack on disk.  This does not erase the file.
func (d *Stack) Close() error {
	return d.rws.(io.Closer).Close()
}

// Len returns the amount of items currently on the stack.
func (d *Stack) Len() int {
	// Use atomic so that getting Len() is independent from our internal Mutex.
	return int(atomic.LoadInt64(&d.length))
}

// Size returns the current size of the stack in bytes on disk.
func (d *Stack) Size() int {
	// Use atomic so that getting Size() is independent from our internal Mutex.
	return int(atomic.LoadInt64(&d.size))
}

// Pop returns the last stored value that was on the stack and puts it in data.  data must be
// a pointer type (even to reference types) and must either be the same type as was
// passed via New() or a pointer to that type.  If ok == false and err == nil,
// it indicates the stack was empty.
// Note: This should work with almost all fixed types and other basic types.
// But I'm sure some ***type thing will break this.  Pointer to fixed value, struct, and
// reference types is all that is supported.
func (d *Stack) Pop(data interface{}) (ok bool, err error) {
	if !d.isPtrToType(data) {
		return false, fmt.Errorf("cannot pop stored data of type %s into type %T", d.storedType, data)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if atomic.LoadInt64(&d.length) == 0 {
		return false, nil
	}

	if err := d.readData(data); err != nil {
		return false, err
	}

	atomic.AddInt64(&d.length, -1)

	return true, nil
}

// readSize() reads the last 8 bytes of the file to pull a size header off.
// It then truncates the file 8 bytes and moves the offset -8 from the end.
// Returns -1 if it can't read from the end.
func (d *Stack) readSize() int64 {
	// Read the last 8 bytes back as an encoded uint64.
	d.rws.Seek(-8, 2)
	b := make([]byte, 8)
	if _, err := d.rws.Read(b); err != nil {
		glog.Errorf("problem reading from disk stack: %s", err)
		return -1
	}

	// Shrink the file by 8 bytes.
	atomic.AddInt64(&d.size, -8)
	d.f.Truncate(int64(d.Size()))
	d.rws.Seek(-8, 2)

	// Return the decoded size.
	return int64(binary.LittleEndian.Uint64(b))
}

// readData calls n := readSize() and then seeks -n from the end of the file
// and asks the gob decoder to read the entry.  It then truncates the file
// the size of the data we read and seeks to the new end position.
func (d *Stack) readData(data interface{}) error {
	n := d.readSize()
	if n < 0 {
		return fmt.Errorf("could not read the size of the next entry on the stack")
	}

	if _, err := d.rws.Seek(-n, 2); err != nil {
		return fmt.Errorf("could not seek back to start of data(-%d bytes): %s", n, err)
	}

	b := make([]byte, n)
	rs, err := d.rws.Read(b)
	if err != nil {
		return fmt.Errorf("data size was %d, but encountered error trying to read that from disk: %s", err)
	}
	if int64(rs) != n {
		d.f.Seek(int64(-rs), 1)
		return fmt.Errorf("data size was %d, but only could read %d back", n, rs)
	}

	//read := byter{d.rws}
	//read := bufio.NewReader(d.rws)
	dec := gob.NewDecoder(bytes.NewBuffer(b))
	if err := dec.Decode(data); err != nil {
		return fmt.Errorf("could not decode the gob data: %s", err)
	}

	atomic.AddInt64(&d.size, -n)
	d.f.Truncate(int64(d.Size()))
	d.rws.Seek(-n, 2)

	return nil
}

// Push pushes an new entry onto the stack.  data must be of the same type
// passed in New().
func (d *Stack) Push(data interface{}) error {
	if !d.isType(data) {
		return fmt.Errorf("cannot push data of type %T into stack of type %s", data, d.storedType)
	}

	buff := bytes.NewBuffer([]byte{})
	enc := gob.NewEncoder(buff)

	// Write our data to the buffer.
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("could not gob encode the data: %s", err)
	}

	// Write our size header.
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(buff.Len()))
	if _, err := buff.Write(b); err != nil {
		return fmt.Errorf("unable to write data size to memory buffer: %s", err)
	}

	// Write the data to disk and update counters.
	d.mu.Lock()
	defer d.mu.Unlock()

	d.rws.Seek(0, 2)
	n, err := d.rws.Write(buff.Bytes())
	if err != nil {
		d.f.Truncate(int64(d.Size()))
		d.rws.Seek(-int64(n), 2)
		return fmt.Errorf("could not write to the file: %s", err)
	}

	atomic.AddInt64(&d.size, int64(n))
	atomic.AddInt64(&d.length, 1)

	if d.flush {
		d.f.Sync()
	}

	return nil
}

func (d *Stack) isPtrToType(data interface{}) bool {
	t := reflect.TypeOf(data)
	if t.Kind() != reflect.Ptr {
		return false
	}

	if t.Elem().Name() != d.storedType {
		return false
	}
	return true
}

func (d *Stack) isType(data interface{}) bool {
	t := reflect.TypeOf(data)
	if t.Name() != d.storedType {
		return false
	}
	return true
}
