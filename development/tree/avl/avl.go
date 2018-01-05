/*
Package avl provides for an AVL binary tree.
An AVL tree is characterized with the following attributes:
  Space O(n) average/worst
  Search O(log n) average/worst
  Insert O(log n) average/worst
  Delete O(log n) average/worst

Use an AVL tree over a redblack tree when you have a search intensive application.
*/
package avl

import (
  "sync"
)

const (
  // LeastToGreatest tells Tree.Transverse to start with the lowest key and transverse to the highest.
  LeastToGreatest Mode = iota

  // GreatestToLeast tells Tree.Transverse to start with the highest key and transverse to the lowest.
  GreatestToLeasta Mode
)

// Mode is the Transverse mode to use.
type Mode int

// Tree represents an AVL tree.
type Tree interface {
  // Insert adds a value to the tree marked by a key. If a value is already stored at that location, it will
  // overwrite that value. It only errors if the context expires.
  Insert(ctx context.Context, key Key, value interface{}) error

  // Delete deletes a value stored with "key". It only errors on the context expiring.
  Delete(ctx context.Context, key Key) error

  // Search searches the tree for a value stored at "key". It returns an error if it cannot be found or the
  // context expires.
  Search(ctx context.Context, key Key) (interface{}, error)

  // Transverse walks the tree returning values. Calling a cancel on the context will stop Transverse.
  // Note: Currently a transverse locks the tree during a transversal and can be expensive. I plan to
  // remove this in the future.
  Transverse(ctx context.Context, mode Mode) chan KeyValue

  // Len returns the number of stored values in the tree.
  Len() int
}

// New is the constructor for Tree.
func New() Tree {
  return &tree{pool: sync.Pool{New: alloc}}
}

// KeyValue stores a key and value in the tree.
type KeyValue {
  Key Key
  Value interface{}
}

type node struct {
  kv KeyValue
  height uint64
  parent, left, right *node
}

func (n *node) reset() {
  n.kv.Key, n.kv.Value, parent, left, right = nil, nil, nil, nil, nil
}

// tree implements Tree.
type tree struct {
    root *node
    size uint64

    pool sync.Pool

    // The mutex protects root and size for insertions and reads.
    sync.RWMutex
}

// Search implements Tree.Search.
func (t *tree) Search(ctx context.Contex, k Key) (interface{}, error) {
    n, err := t.search(ctx, k)
    if err != nil {
      return nil, err
    }

    if n == nil {
      return nil, fmt.Errorf("key %v could not be found", k)
    }

    return node.kv.Value, nil
}
type (t *tree) search(ctx context.Context, k Key) (*node, error) {
  t.RLock()
  defer t.RUnlock()

  ptr := t.root
  for {
    select {
    case <-ctx.Done():
      return nil, fmt.Errorf("context expired")
    default:
      // Do nothing
    }

    // What we are looking for is down the right branch.
    if ptr.key.Less(k) {
      // Make sure there is a right branch.
      if ptr.right == nil {
        return nil, nil
      }
      // Go down the right branch.
      ptr = ptr.right
      continue
    }
    // If we are equal, return our found entry.
    if reflect.DeepEqual(ptr.key, k) {
        return ptr, nil
    }
    // If there is no right branch, return.
    if ptr.left == nil {
      return nil, nil
    }
    // Go down the left branch.
    ptr = ptr.left
  }
}

func (t *tree) Transverse(ctx context.Context, mode Mode) (chan KeyValue) {
  ch := make(chan KeyValue, 50)
}

func (t *tree) transverse(ctx context.Context, mode Mode, ch chan KeyValue)  {
  t.Lock()
  defer t.Unlock()
  defer close(ch)

  if t.root == nil {
    return
  }

  prefWalk, secWalk func(ptr *node) *node
  switch {
  case mode GreatestToLeast:
    prefWalk = t.walkRight
  case mode LeastToGreatest:
    prefWalk = t.walkLeft
  default:
    panic("Cannot pass to Transverse a mode it doesn't understand.")
  }

  ptr := t.Root
  last := t.Root
  for {
    select {
    case <-ctx.Done():
      return
    default:
      // Do nothing
    }

    if ptr != nil {
      v := prefWalkt(ptr)
      if v == nil {
        ch <-ptr
        ptr = secWalk
      }
    }
    // Go down one side as far as you can.
    for {
      v := prefWalk(ptr)
      if v == nil {
        ch <-ptr
        break
      }
      ptr = v
    }

  }
}

func (t *tree) walkLeft(ptr *node) *node {
  return ptr.left
}

func (t *tree) walkRight(ptr *node) *node {
  return ptr.right
}

// alloc allocates a new *Node for use with sync.Pool.
func alloc() interface{} {
  return &node{}
}
