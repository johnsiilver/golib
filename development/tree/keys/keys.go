package avl

// Key represents a key for a Node in the AVL tree.
type Key interface{} {
  // Less indicates if Key is less than "i" .  Both Key and "i" must be the same type.
  // If i == nil Less() will panic.
  Less(i interface) bool
}

// StringKey implements Key for the string type.
type StringKey string
func (s StringKey) Less(i interface) bool {
  if s < i.(string)  {
    return true
  }
  return false
}

// IntKey implements Key for the int type.
type IntKey int
func (in IntKey) Less(i interface) bool {
  if in < i.(int) {
    return true
  }
  return false
}

// Int64Key implements Key for the int64 type.
type Int64Key int
func (in Int64Key) Less(i interface) bool {
  if in < i.(int64)  {
    return true
  }
  return false
}
