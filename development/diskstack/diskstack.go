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
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"os"
	"reflect"
	"sync"
  "sync/atomic"

	"github.com/golang/glog"
)

// Stack implements a LIFO queue that can store a single object type that can
// be encoded by the gob encoder (encoding/gob) on local disk.
// The file is constantly resized based for each pop() and push() operation.
// All operations are thread-safe.
type Stack struct {
	storedType string
	f      *os.File

  mu     sync.Mutex // Protects everything below.
	length int64  // Only access with atomic operations.
	size   int64  // Dito.
}

// New creates a new instance of Stack.  p is the location of where to write the
// stack.  The file must not exist.  dataType is the value that you will store
// in the stack.  It must be gob encodable.  Also see Pop() for more information
// on what can go here.
func New(p string, dataType interface{}) (*Stack, error) {
	f, err := os.OpenFile(p, os.O_RDWR+os.O_CREATE+os.O_EXCL, 0666)
	if err != nil {
		return nil, err
	}

	return &Stack{f: f, length: 0, storedType: reflect.TypeOf(dataType).Name()}, nil
}

// Close closes the file backing the stack on disk.  This does not erase the file.
func (d *Stack) Close() error {
	return d.f.Close()
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

// Pop returns the last data that was on the stack and puts it in data.  data must be
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
	d.f.Seek(-8, 2)
	b := make([]byte, 8)
	if _, err := d.f.Read(b); err != nil {
		glog.Errorf("problem reading from disk stack: %s", err)
		return -1
	}

	// Shrink the file by 8 bytes.
	d.f.Seek(-8, 2)
	atomic.AddInt64(&d.size, -8)

	d.f.Truncate(int64(d.Size()))

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

	d.f.Seek(-n, 2)
	read := bufio.NewReader(d.f)
	dec := gob.NewDecoder(read)
	if err := dec.Decode(data); err != nil {
		return fmt.Errorf("could not decode the gob data: %s", err)
	}

	atomic.AddInt64(&d.size,  -n)
	d.f.Truncate(int64(d.Size()))
	d.f.Seek(-n, 2)

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

	d.f.Seek(0, 2)
	n, err := d.f.Write(buff.Bytes())
	if err != nil {
		d.f.Truncate(int64(d.Size()))
		d.f.Seek(-int64(n), 2)
		return fmt.Errorf("could not write to the file: %s", err)
	}

  atomic.AddInt64(&d.size, int64(n))
  atomic.AddInt64(&d.length, 1)

	d.f.Sync()
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
