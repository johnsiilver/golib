/*
Package lru implements a Least Recently Used cache.

This implmenetation lets you set the limitations of the cache size based upon a number of entries.

The package also includes a utility called "tailor" for tailoring the cache to a specific key/value type
instead of interface{}, allowing you to remove the overhead of runtime type inference.
*/
package lru

import (
  "fmt"
  "sync"
)

type node struct {
  k interface{}
  v interface{}
  next *node
  prev *node
}

// Cache provides the LRU cache.
type Cache interface{
  // Get returns the value stored at key "k" and true if found. O(1) operation.
  Get(k interface{}) (interface{}, bool)

  // Set adds a value at key "k" of value "v". O(1) operation.
  Set(k interface{}, v interface{}) error

  // Remove removes a value at key "k". O(1) operation.
  Remove(k interface{})

  // RemoveOldest removes the oldest value in the cache. O(1) operation.
  RemoveOldest()

  // Len returns the current length of the cache.
  Len() int
}

// cache implements Cache.
type cache struct {
  cache map[interface{}]*node
  startList *node
  endList *node
  preSize uint64
  sync.RWMutex

  numLimit int
}

// Option is an optional argument for New().
type Option func(c *cache)

// NumberLimit sets a limit to the number of items that can be stored in the Cache before
// an insertion purges an existing item.
func NumberLimit(i int) Option {
  return func(c *cache) {
    c.numLimit = i
  }
}

// PreSize pre-sizes the internal data structures to hold "i" items. Similar to make(map[key]value, i).
func PreSize(i uint64) Option {
  return func(c *cache) {
    c.preSize = i
  }
}

// New is the constructor for Cache. Must pass either NumberLimit or MemoryLimit, but not both.
func New(options ...Option) (Cache, error) {
    c := &cache{}

    for _, opt := range options {
      opt(c)
    }

    if c.numLimit == 0 {
      return nil, fmt.Errorf("must provide the NumberLimit() option. Yes its an option for future proofing.")
    }

    c.cache = make(map[interface{}]*node, c.preSize)
    return c, nil
}

// Get implements Cache.Get().
func (c *cache) Get(k interface{}) (interface{}, bool) {
  c.RLock()
  defer c.RUnlock()

  n, ok := c.cache[k]
  if !ok {
    return nil, false
  }

  // No need to move anything if its already at the end of the list.
  if n.k == c.endList.k {
    return n.v, true
  }

  c.removeList(k)
  n.next = nil
  n.prev = nil
  c.addList(n)

  return n.v, true
}

// Set implements Cache.Set().
func (c *cache) Set(k interface{}, v interface{}) error {
  c.Lock()
  defer c.Unlock()

  return c.setNum(k, v)
}

func (c *cache) setNum(k interface{}, v interface{}) error {
  // That key already exists, so just do a value replacement.
  if _, ok := c.cache[k]; ok {
    c.cache[k].v = v
    return nil
  }

  // Cache limit was hit, so remove the last used item.
  if len(c.cache)+1 > c.numLimit {
    removeKey := c.startList.k
    c.removeList(removeKey)
    delete(c.cache, removeKey)
  }

  n := &node{k: k, v: v}
  c.addList(n)
  c.cache[k] = n

  return nil
}

// add adds a node to the end of the list.
func (c *cache) addList(n *node) {
  // This would indicate our list is empty.
  if c.endList == nil {
    c.startList = n
    c.endList = n
    return
  }

  // Add our node to the end of the list.
  c.endList.next = n
  n.prev = c.endList
  c.endList = n
}

// Remove implements Cache.Remove().
func (c *cache) Remove(k interface{}) {
  c.Lock()
  defer c.Unlock()

  c.removeList(k)
  delete(c.cache, k)
}

func (c *cache) removeList(k interface{}) {
  n, ok := c.cache[k]
  if !ok {
    return
  }

  // This sections removes the node from the linked list and sets the next/prev pointers
  // for the nodes before and after our entry to their new neighbors.
  if n.next != nil {
    n.next.prev = n.prev
  }

  if n.prev != nil {
    n.prev.next = n.next
  }

  // If this node was the root node, change the root node to the next node.
  if c.startList.k == n.k {
    c.startList = n.next
  }

  // If this node was the end node, change the end node to the prev node.
  if c.endList.k == n.k {
    c.endList = n.prev
  }

  return
}

// RemoveOldest implements Cache.RemoveOldest().
func (c *cache) RemoveOldest() {
  c.Remove(c.startList.k)
}

// Len implements Cache.Len().
func (c *cache) Len() int {
  c.RLock()
  defer c.RUnlock()
  return len(c.cache)
}
