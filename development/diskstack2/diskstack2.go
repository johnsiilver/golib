/*
Package diskstack2 implements a LIFO queue (known as a stack) on local disk.
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
package diskstack2

import (
	//"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/johnsiilver/golib/mmap"
)

// Stack implements a LIFO queue that can store a single object type that can
// be encoded by the gob encoder (encoding/gob) on local disk.
// The file is constantly resized based for each pop() and push() operation.
// All operations are thread-safe.
type Stack struct {
	storedType string
	path       string

	mu     sync.Mutex // Protects everything below.
	id     int64
	length int64 // Only access with atomic operations.
}

// New creates a new instance of Stack.  p is the location of where to write the
// stack.  The file must not exist.  dataType is the value that you will store
// in the stack.  It must be gob encodable.  Also see Pop() for more information
// on what can go here.
func New(p string, dataType interface{}) (*Stack, error) {

	s := &Stack{path: p, id: -1, storedType: reflect.TypeOf(dataType).Name()}

	return s, nil
}

// Close closes the file backing the stack on disk.  This does not erase the file.
func (d *Stack) Close() error {
	return nil
}

// Len returns the amount of items currently on the stack.
func (d *Stack) Len() int {
	// Use atomic so that getting Len() is independent from our internal Mutex.
	return int(atomic.LoadInt64(&d.length))
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

	f, err := os.OpenFile(path.Join(d.path, strconv.Itoa(int(atomic.LoadInt64(&d.id)))), os.O_RDONLY, 0666)
	if err != nil {
		return false, err
	}

	m, err := mmap.NewMap(f, mmap.Prot(mmap.Read), mmap.Prot(mmap.Write), mmap.Flag(mmap.Private))
	if err != nil {
		return false, err
	}

	buff := bytes.NewBuffer(nil)
	if _, err := io.Copy(buff, m); err != nil {
		return false, fmt.Errorf("Pop(): could not read file into buffer: %s", err)
	}
	f.Close()

	dec := gob.NewDecoder(buff)
	if err := dec.Decode(data); err != nil {
		return false, fmt.Errorf("Pop(): could not decode entry: %s", err)
	}

	// TODO(johnsiilver): Maybe add some logging.
	os.Remove(path.Join(d.path, strconv.Itoa(int(atomic.LoadInt64(&d.id)))))

	atomic.AddInt64(&d.length, -1)
	atomic.AddInt64(&d.id, -1)

	return true, nil
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

	// Write the data to disk and update counters.
	d.mu.Lock()
	defer d.mu.Unlock()

	f, err := os.OpenFile(path.Join(d.path, strconv.Itoa(int(atomic.LoadInt64(&d.id))+1)), os.O_RDWR+os.O_CREATE+os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(buff.Bytes()); err != nil {
		return err
	}
	f.Sync()
	f.Seek(0, 0)

	atomic.AddInt64(&d.length, 1)
	atomic.AddInt64(&d.id, 1)

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
