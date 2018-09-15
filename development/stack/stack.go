// Package stack provides implementations that stacks in lower level libraries
// must implement.
package stack

// Byte implemnents a stack that stores []bytes.  Every stack must implement this.
type Byte interface {
  // Pop an entry off the stack.  Will return stack.Empty if nothing to pop.
  Pop() ([]byte, error)
  // Push an entry onto the stack.
  Push(b []byte) error
  // Pull is a Pop that will wait for an entry to be available.
  Pull() ([]byte, error)
  // Len returns the length of the stack.
  Len() int
  // Size returns the size in bytes of the stack.
  Size() int64
}

// Encoded provides a stack that uses an encoding method, like encoding/gob,
// encoding/json, protocol buffers, etc... to encode data we want to
// Push/Pop onto the stack.  You can only store a single type in the stack.
type Encoded interface {
  // EncPop will pop an entry off the stack and put it in "i".  "i" must be a
  // pointer, even if a reference type.  Will return stack.Empty if nothing to
  // pop.
  EncPop(i interface{}) error
  // EncPush will push "i" onto the stack.
  EncPush(i interface{}) error
  // EncPull is a GobPop that will wait for an entry to be available.
  EncPull(i interface{}) error
}
