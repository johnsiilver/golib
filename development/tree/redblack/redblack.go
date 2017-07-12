// Package redblack provides a library for using a redblack tree. Based on code at:
// https://www.cs.auckland.ac.nz/software/AlgAnim/red_black.html
// This library is thread-safe.
package redblack

import (
  "sync"
)

const (
  red = false
  black = true
)

// color is used to indicate the color of a node.
type color bool

// Key provides a method for determine if a value is less than another value.
type Key interface{
  // LessThan indicates that Key is less than LessThan(value).
  LessThan(interface{}) bool
}

type node struct {
    color color
    key Key
    item interface{}
    parent, left, right *node
}

type tree struct {
  root *node
  size int
  sync.RWMutex
}

// Insert inserts value "v" for key "k".
func (t *tree) Insert(k Key, v interface{}) {
  t.Lock()
  defer t.Unlock()
  defer func(){t.size++}()

  newNode := &node{
    color: black,
    key: k,
    item: v,
  }

  t.treeInsert(newNode)
  t.insertRebalance(newNode)
}

// treeInsert does a standard binary tree insert, ignoring red/black properties.
func (t *tree) treeInsert(newNode *node) {
  defer func(){t.size++}()
  if t.root == nil {
    t.root = newNode
    return
  }

  newNode.color = red
  newNode.parent = t.root
  n := t.root
  for {
    // Branch right.
    if n.key.LessThan(newNode.key) {
      if n.right == nil {
        n.right = newNode
        break
      }
      n = n.right
      newNode.parent = n
      continue
    }else{
      if n.left == nil {
        n.left = newNode
        break
      }
      n = n.left
      newNode.parent = n
      continue
    }
  }
}

func (t *tree) insertRebalance(n *node) {
  for n != t.root && n.parent.color == red {
    if n.parent == n.parent.parent.left {
      // If x's parent if a left, y is x's right uncle.
      y := n.parent.parent.right
      if y.color == red {
        // Change the colors.
        n.parent.color = black
        y.color = black
        n.parent.parent.color = red

        // Move n up the tree.
        n = n.parent.parent
      }else {
        if n == n.parent.right {
          n = n.parent
          t.leftRotate(n)
        }
        n.parent.color = black
        n.parent.parent.color = red
        t.rightRotate(n.parent.parent)
      }
    }else{
      // If x's parent if a left, y is x's right uncle.
      y := n.parent.parent.left
      if y.color == red {
        // Change the colors.
        n.parent.color = black
        y.color = black
        n.parent.parent.color = red

        // Move n up the tree.
        n = n.parent.parent
      }else {
        if n == n.parent.left {
          n = n.parent
          t.rightRotate(n)
        }
        n.parent.color = black
        n.parent.parent.color = red
        t.leftRotate(n.parent.parent)
      }
    }
  }
  t.root.color = black
}

func (t *tree) leftRotate(n *node) {
  parent := n.parent

  y := n.right
  z := y.left
  y.parent = n.parent
  n.parent = y

  // Perform rotation
  y.left = n
  n.right = z

  if parent == nil {
    t.root = y
    return
  }

  if parent.left == n {
    parent.left = y
  }else {
    parent.right = y
  }
}

func (t *tree) rightRotate(n *node) {
  parent := n.parent

  y := n.left
  z := y.right
  y.parent = n.parent
  n.parent = y

  // Perform rotation
  y.right = n
  n.left = z

  if parent == nil {
    t.root = y
    return
  }

  if parent.left == n {
    parent.left = y
  }else {
    parent.right = y
  }
}
