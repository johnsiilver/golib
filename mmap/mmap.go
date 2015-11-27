/*
Package mmap exposes the Unix mmap calls with helper utilities.  mmap provides a memory caching
scheme for files on disk that may save memory when shared between processes or save memory when
doing random access. You can read more about mmap in the linux man pages.  This package currently
only works for Unix based systems.

The most basic type to use is a Map object.  It can be created as follows:
  // Create a file to be mmapped or in open a file with os.Open().
  f, err := ioutil.TempFile("", "")
  if err != nil {
         panic(err)
  }

  // Because we are creating a new file, give it some content as you can't mmap an empty file.
  _, err = io.WriteString(f, "hello world")
  if err != nil {
         panic(err)
  }

  // Create an mmapped file that can be read, written, and shared between processes.
  m, err := NewMap(f, Prot(Read), Prot(Write), Flag(Shared))
  if err != nil {
    // Do something
  }
  defer m.Close()

  // You now have access to all the methods in io.Reader/io.Writer/io.Seeker()/io.ReaderAt/io.Closer.
  p := make([]byte, 5)
  _, err := m.Read(p)
  if err != nil {
    // Do something
  }

  // You can access the Map's []byte directly.  However, there is no goroutine mutexes or other protections
  // that prevent faults (not panic, good old C like faults).
  fmt.Println(m.Bytes()[5:])
}
*/
package mmap

import (
  "fmt"
  "io"
  "log"
  "os"
  "sync"
  "syscall"
)

// Values that can be used for the Prot() arg to New().
const (
  // Read indicates the data can be read.
  Read = syscall.PROT_READ

  // Write indicates the data can be written.
  Write = syscall.PROT_WRITE

  // Exec indicates the data can be executed.
  Exec = syscall.PROT_EXEC
)

// Values that can be used with the Flag() option to New().
const (
  // Shared indicates changes to the data is shared between processes.  Shared and Private cannot be set together.
  Shared = syscall.MAP_SHARED

  // Private indicates changes to the data is for the current process only and will not change the underlying object.
  // Private and Shared cannot be used together.
  Private = syscall.MAP_PRIVATE
)

// Map represents a mapped file in memory and implements the io.ReadWriteCloser/io.Seeker/io.ReaderAt interfaces.
// Note that any change to the []byte returned by various methods is changing the underlying memory representation
// for all users of this mmap data.  For safety reasons or when using concurrent access, use the built in methods
// to read and write the data
type Map interface {
  io.ReadWriteCloser
  io.Seeker
  io.ReaderAt

  // Bytes returns the bytes in the map. Modifying this slice modifies the inmemory representation.
  // This data should only be used as read-only and instead you should use the Write() method that offers
  // better protections.  Write() will protect you from faults like writing to a read-only mmap or other
  // errors that cannot be caught via defer/recover. It also protects you from writing data that cannot
  // be sync'd to disk because the underlying file does not have the capactiy (regardless to what you
  // set the mmap length to).
  Bytes() []byte

  // Len returns the size of the file, which can be larger than the memory mapped area.
  Len() int

  // Pos returns the current index of the file pointer.
  Pos() int

  // Lock prevents the physical memory from being swapped out to disk.
  Lock() error

  // Unlock allows the physical memory to be swapped out to disk. If the memory is not locked, nothing happens.
  Unlock() error
}

/*
type String interface {
  io.ReadWriter
  io.Seeker
  io.Closer
  ReadFile() ([]string, error)
  ReadString(delim byte) (line string, err error)
  ReadLine() (string, error)
  String() ([]string, error)
}

type Structured interface {
  io.Closer
  // Marshal attempts to write "t" into the mapped file. This will always be written from the start of the file.
  // Must have set MarshalFunc() to set a marshalling function.
  Marshal(t interface{}) error

  // Unmarshal attempts to read the mapped file into "t". Must have used UnmarshalFunc() to set an unmarshaling function.
  Unmarshal(t interface{}) error

  // MarshalFunc sets a function that is called to marshal a data structure represented by "t" into bytes. This is used
  // whenenver Unmarshal is called.
  MarshalFunc(f func(t interface{}) ([]byte, error))

  // UnmarshalFunc sets a function that is called to unmarshal the underlying file contents into a data structure
  // represented by t. This is used whenever Marshal() is called.
  UnmarshalFunc(f func(t interface{}) error)
}
*/

// Option is an option to the New() constructor.
type Option func(m *mmap)

// Prot allows you to pass a Prot value that will be bitwise OR'd to come up with the final value.
// Only use the predefined constants.
func Prot(p int) Option {
  return func(m *mmap) {
    if p == Write {
      m.write = true
    }

    if m.prot != -1 {
      m.prot = m.prot | p
      return
    }
    m.prot = p
  }
}

// Flag sets the flag value for the mmap call. Only use the predefined constants.
func Flag(f int) Option {
  return func(m *mmap) {
    m.flags = f
  }
}

// Anon indicates that the memory should not be backed by a file. In this case,
// the fd and length arguments are ignored by New().
func Anon() Option {
  return func(m *mmap) {
    m.anon = true
  }
}

// Length sets the length in bytes of the file from the offset that we are mapping to memory.
// Unless Offset() is called, this is from the beginning of the file.  You can use this to provide
// extra room for the file to grow.
func Length(s int) Option {
  return func(m *mmap) {
    m.len = s
  }
}

// Offset is where to start the mapping from. Must be a multiple of the system's page size.
// By default the entire file is mapped.
func Offset(o int64) Option {
  return func(m *mmap) {
    m.offset = o
  }
}

// NewMap creates a new Map object that provides methods for interacting with the mmap'd file.
func NewMap(f *os.File, opts ...Option) (Map, error) {
  return newMap(f, opts...)
}

func newMap(f *os.File, opts ...Option) (*mmap, error) {
  m := &mmap{
    flags: -1,
    prot: -1,
    len: -1,
  }

  for _, opt := range opts {
    opt(m)
  }

  if m.flags == -1 || m.prot == -1 {
    return nil, fmt.Errorf("must pass options to set the flag or prot values")
  }

  if f == nil && !m.anon{
    return nil, fmt.Errorf("f arg cannot be nil, anon was %v", m.anon)
  }

  var fd uintptr

  if m.anon {
    fd = ^uintptr(0)
    m.flags = m.flags | syscall.MAP_ANON
    if m.len <= 0 {
      return nil, fmt.Errorf("must use Length() if using Anon() option")
    }
  }else{
    fd = f.Fd()

    s, err := f.Stat()
    if err != nil {
      return nil, err
    }

    if s.Size() == 0 {
      return nil, fmt.Errorf("cannot mmap 0 length file")
    }

    if m.len == -1 {
      m.len = int(s.Size())
    }
  }

  var err error
  if m.anon {
    m.data, err = syscall.Mmap(-1, 0, m.len, m.prot, m.flags)
  }else{
    m.data, err = syscall.Mmap(int(fd), m.offset, m.len, m.prot, m.flags)
  }

  if err != nil {
    return nil, fmt.Errorf("problem with mmap system call: %q", err)
  }

  return m, nil
}

// mmap implements Map.
type mmap struct {
  flags, prot, len int
  offset int64
  anon bool
  data []byte
  ptr int
  write bool

  sync.RWMutex
}

// Bytes implements Map.Bytes().
func (m *mmap) Bytes() []byte {
  m.RLock()
  defer m.RUnlock()

  return m.data
}

// Len returns the size of the file, which can be larger than the memory mapped area.
func (m *mmap) Len() int {
  return m.len
}

// Read implements io.Reader.Read().
func (m *mmap) Read(p []byte) (int, error) {
  m.RLock()
  defer m.RUnlock()

  if m.ptr >= m.len {
    log.Println(m.len)
    return 0, io.EOF
  }

  n := copy(p, m.data[m.ptr:])
  m.ptr += n

  if n == m.len - m.ptr {
    return n, io.EOF
  }

  return n, nil
}

// ReadAt implements ReaderAt.ReadAt().
func (m *mmap) ReadAt(p []byte, off int64) (n int, err error) {
  m.RLock()
  defer m.RUnlock()

  if int(off) >= m.len {
    return 0, fmt.Errorf("offset is larger than the mmap []byte")
  }

  n = copy(p, m.data[off:])
  if n < len(p) {
    return n, fmt.Errorf("len(p) was greater than mmap[off:]")
  }
  return n, nil
}

// Write implements io.Writer.Write().
func (m *mmap) Write(p []byte) (n int, err error) {
  m.Lock()
  defer m.Unlock()
  if !m.write {
    return 0, fmt.Errorf("cannot write to non-writeable mmap")
  }

  if len(p) > m.len - m.ptr {
    return 0, fmt.Errorf("attempting to write past the end of the mmap'd file")
  }

  n = copy(m.data[m.ptr:], p)
  m.ptr += n
  return n, nil
}

// Seek implements io.Seeker.Seek().
func (m *mmap) Seek(offset int64, whence int) (int64, error) {
  if offset < 0 {
    return 0, fmt.Errorf("cannot seek to a negative offset")
  }

  m.Lock()
  defer m.Unlock()

  switch whence {
  case 0:
    if offset < int64(m.len) {
      m.ptr = int(offset)
      return int64(m.ptr), nil
    }
    return 0, fmt.Errorf("offset goes beyond the data size")
  case 1:
    if m.ptr + int(offset) < m.len {
      m.ptr += int(offset)
      return int64(m.ptr), nil
    }
    return 0, fmt.Errorf("offset goes beyond the data size")
  case 2:
    if m.ptr - int(offset) > -1 {
      m.ptr -= int(offset)
      return int64(m.ptr), nil
    }
    return 0, fmt.Errorf("offset would set the offset as a negative number")
  }
  return 0, fmt.Errorf("whence arg was not set to a valid value")
}

// Pos implements Map.Pos().
func (m *mmap) Pos() int {
  m.RLock()
  defer m.RUnlock()

  return m.ptr
}

// Lock implements Map.Lock().
func (m *mmap) Lock() error {
  return syscall.Mlock(m.data)
}

// Unlock implements Map.Unlock().
func (m *mmap) Unlock() error {
  m.RLock()
  defer m.RUnlock()

  return syscall.Munlock(m.data)
}

// Close implements  Map.Close().
func (m *mmap) Close() error {
  m.RLock()
  defer m.RUnlock()

  return syscall.Munmap(m.data)
}
