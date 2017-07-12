package redblack

// Int implements Key for the int type.
type Int int

// LessThan implements Key.LessThan.
func (x Int) LessThan(i interface{}) bool {
  if int(x) < i.(int) {
    return true
  }
  return false
}

// String implements Key for the string type.
type String int

// LessThan implements Key.LessThan.
func (x String) LessThan(i interface{}) bool {
  if string(x) < i.(string) {
    return true
  }
  return false
}
