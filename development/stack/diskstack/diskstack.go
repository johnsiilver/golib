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

  fmt.Println("stack size in bytes: %d", s.Size())
*/
package diskstack

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

// int64Size is the []byte size of encoding the size of the next gob data.
const int64Size = 8

var (
	// StackFull indicates that MaxDepth was reached and no entries can be added
	// until a Pop() occurs.
	StackFull = errors.New("stack has reached its max depth")
	// StackEmpty indicates the stack had no entries.
	StackEmpty = errors.New("stack was empty")
)

// Stack implements a LIFO queue that can store a single object type that can
// be encoded by the gob encoder (encoding/gob) on local disk.
// The file is constantly resized based for each pop() and push() operation.
// All operations are thread-safe.
type Stack struct {
	storedType    string
	storedReflect reflect.Value
	f             *os.File
	rws           io.ReadWriteSeeker

	// Options passed via the constructor.
	flush        bool
	existingPath bool
	maxDepth     int

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

// UseExisting indicates that "p" exists with an existing queue.
func UseExisting() Option {
	return func(s *Stack) {
		s.existingPath = true
	}
}

// MaxDepth indicates how many entries can be in the Stack.  If d <= 0, this
// is limited only by disk space.
func MaxDepth(d int) Option {
	return func(s *Stack) {
		s.maxDepth = d
	}
}

// New creates a new instance of Stack.  p is the location of where to write the
// stack.  The file must not exist unless UseExisting() is passed.
// dataType is the value that you will store in the stack.  It must be gob
// encodable.  See Pop() for more information on what can be stored.
func New(p string, dataType interface{}, options ...Option) (*Stack, error) {
	s := &Stack{flush: true, storedType: reflect.TypeOf(dataType).Name(), storedReflect: reflect.ValueOf(dataType)}
	for _, o := range options {
		o(s)
	}

	if s.existingPath {
		if err := s.existingFile(p); err != nil {
			return nil, err
		}
	} else {
		if err := s.newFile(p); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (d *Stack) newFile(p string) error {
	f, err := os.OpenFile(p, os.O_RDWR+os.O_CREATE+os.O_EXCL, 0666)
	if err != nil {
		return err
	}

	vi := VersionInfo{Version: internalVersion, Created: time.Now().Unix()}
	viLen, err := vi.encode(f)
	if err != nil {
		return err
	}

	if _, err := f.Seek(int64(viLen), 0); err != nil {
		return fmt.Errorf("cannot seek past the VersionInfo header we wrote")
	}

	d.f = f
	d.rws = f
	d.size = int64(viLen)
	return nil
}

func (d *Stack) existingFile(p string) error {
	panic("please implement")

	stat, err := os.Stat(p)
	if err != nil {
		fmt.Errorf("could not stat the file at %s: %s", p, err)
	}

	f, err := os.OpenFile(p, os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	vi := &VersionInfo{}
	n, err := vi.decode(f)
	if err != nil {
		return err
	}

	d.f = f
	d.rws = f
	d.size = stat.Size()

	if err := d.discoverLength(int64(n)); err != nil {
		return fmt.Errorf("while trying to read %s: %s", p, err)
	}

	if _, err := d.rws.Seek(0, 2); err != nil {
		return fmt.Errorf("%s: could not seek to end of file: %s", p, err)
	}

	return nil
}

// discoverLength reads through all entries in an existing file stack to make
// sure that all entries can be decoded and sets our stack's current length.
func (d *Stack) discoverLength(versionHeaderSize int64) error {
	if _, err := d.rws.Seek(0, 2); err != nil {
		return fmt.Errorf("could not seek to end of file: %s", err)
	}

	// Get a copy of the data type we are storing.  We must be able to
	// retrieve each entry and this allows os to validate that.
	var data reflect.Value
	switch d.storedReflect.Type().Kind() {
	case reflect.Ptr, reflect.Map:
		data = reflect.New(d.storedReflect.Type().Elem())
	default:
		data = reflect.New(d.storedReflect.Type())
	}

	var pos int64
	var length int64
	for (d.size - int64(versionHeaderSize)) > pos {
		n, err := d.readData(data.Interface(), false)
		if err != nil {
			return fmt.Errorf("while trying to verify the file, entry %d had error: %s", length, err)
		}
		pos += n
		length++
	}
	d.length = length
	return nil
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

// Pop returns the last stored value that was on the stack and puts it in data.
// data must be a pointer type (even to reference types) and must either be the
// same type as was passed via New() or a pointer to that type.  Structs will
// only have Public fields stored (gob restriction).
// If ok == false and err == nil, it indicates the stack was empty.
// Note: This should work with almost all fixed types and other basic types.
// But I'm sure some ***type thing will break this.  Pointer to fixed value, struct, and
// reference types is all that is supported.
func (d *Stack) Pop(data interface{}) error {
	if !d.isPtrToType(data) {
		return fmt.Errorf("cannot pop stored data of type %s into type %T", d.storedType, data)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if atomic.LoadInt64(&d.length) == 0 {
		return StackEmpty
	}

	if _, err := d.readData(data, true); err != nil {
		return err
	}

	atomic.AddInt64(&d.length, -1)

	return nil
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
func (d *Stack) readData(data interface{}, truncate bool) (n int64, err error) {
	n = d.readSize()
	if n < 0 {
		return 0, fmt.Errorf("could not read the size of the next entry on the stack")
	}

	// Seek to the beginning of the gob.
	if _, err := d.rws.Seek(-n, 2); err != nil {
		return 0, fmt.Errorf("could not seek back to start of data(-%d bytes): %s", n, err)
	}

	// Read the gob.
	b := make([]byte, n)
	rs, err := d.rws.Read(b)
	if err != nil {
		return 0, fmt.Errorf("data size was %d, but encountered error trying to read that from disk: %s", len(b), err)
	}

	// This would indicate that our entry header said we had data of size x, but found we had data of size y.
	if int64(rs) != n {
		d.f.Seek(int64(n-int64(rs))+int64Size, 1) // Move us back before the entry + header.
		return 0, fmt.Errorf("data size was %d, but only could read %d back", n, rs)
	}

	dec := gob.NewDecoder(bytes.NewBuffer(b))
	if err := dec.Decode(data); err != nil {
		d.f.Seek(int64Size, 1) // Move us back before the header.
		return 0, fmt.Errorf("could not decode the gob data: %s", err)
	}

	if truncate {
		// Update the size to indicate we removed the entry.
		atomic.AddInt64(&d.size, -n)

		// Remove the entry.
		d.f.Truncate(int64(d.Size()))
	}

	// Move the file index to the beginning of the gob.
	d.rws.Seek(-n, 2)

	return n + int64Size, nil
}

// Push pushes an new entry onto the stack.  data must be of the same type
// passed in New().
func (d *Stack) Push(data interface{}) error {
	if !d.isType(data) {
		return fmt.Errorf("cannot push data of type %T into stack of type %s", data, d.storedType)
	}

	if d.maxDepth > 0 && d.Len() >= d.maxDepth {
		return StackFull
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
