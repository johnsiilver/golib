// Package ordered provides a map that maintains insertion order.
// This is a simple linked list implementation, which means we double our key space.
// There are no duplicate values, but we now store 2 pointers per entry.
//
// Note: I considered using https://github.com/bahlo/generic-list-go/blob/main/list.go
// Which is a great library. However, it has a lot of unneccsary stuff around JSON and
// imports things I don't want to import.
package ordered

import (
	"context"
)

type entry[K comparable, V any] struct {
	key  K
	val  V
	prev *entry[K, V]
	next *entry[K, V]
}

func (e *entry[K, V]) remove() {
	if e.prev != nil {
		e.prev.next = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	}
}

func (e *entry[K, V]) add(a *entry[K, V]) {
	if e.next != nil {
		panic("pair already added")
	}
	e.next = a
	a.prev = e
}

// Map is an ordered map.
type Map[K comparable, V any] struct {
	m map[K]*entry[K, V]

	front *entry[K, V]
	end   *entry[K, V]
}

// New returns a new ordered map.
func New[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		m: make(map[K]*entry[K, V]),
	}
}

// KV is a result for a Range.
type KV[K comparable, V any] struct {
	// Key is the key.
	Key K
	// Value is the value.
	Value V
	// Err is the error if it has one.
	Err error
}

// Get returns the value for the given key.
func (m *Map[K, V]) Get(key K) (V, bool) {
	p, ok := m.m[key]
	if !ok {
		var v V
		return v, false
	}
	return p.val, true
}

// Get the oldest key and value. If the map is empty, it returns the zero value for both.
func (m *Map[K, V]) Oldest() (K, V) {
	if m.front == nil {
		var k K
		var v V
		return k, v
	}
	return m.front.key, m.front.val
}

// Set sets the value for the given key.
func (m *Map[K, V]) Set(key K, val V) {
	p, ok := m.m[key]
	if ok {
		p.val = val
		return
	}
	p = &entry[K, V]{
		key: key,
		val: val,
	}
	m.m[key] = p

	if m.front == nil {
		m.front = p
		m.end = p
		return
	}
	m.end.add(p)
	m.end = p
}

// Del deletes the value for the given key.
func (m *Map[K, V]) Del(key K) {
	p, ok := m.m[key]
	if !ok {
		return
	}
	delete(m.m, key)

	if p == m.front && p == m.end {
		m.front = nil
		m.end = nil
		return
	}

	next := p.next
	prev := p.prev
	p.remove()

	if p == m.front {
		m.front = next
	}
	if p == m.end {
		m.end = prev
	}
}

// Range ranges over the map in insertion order.
func (m *Map[K, V]) Range(ctx context.Context) chan KV[K, V] {
	ch := make(chan KV[K, V], 1)
	go func() {
		defer close(ch)
		for p := m.front; p != nil; p = p.next {
			select {
			case <-ctx.Done():
				ch <- KV[K, V]{Err: ctx.Err()}
				return
			case ch <- KV[K, V]{Key: p.key, Value: p.val}:
			}
		}
	}()
	return ch
}

// Len returns the length of the map.
func (m *Map[K, V]) Len() int {
	return len(m.m)
}
