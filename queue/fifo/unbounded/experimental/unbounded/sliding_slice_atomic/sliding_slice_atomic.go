package sliding_slice_atomic

import (
	"runtime"
	"sync/atomic"
)

const (
	unlocked = int32(0)
	locked   = int32(1)
)

// Buffer provides a first in first out non-blocking (but not lock free) Buffer.
type Buffer struct {
	nextCh chan interface{}
	data   []interface{}

	lockInt int32
}

// New is the contructor for Buffer.
func New() *Buffer {
	return &Buffer{nextCh: make(chan interface{}, 1)}
}

// Push adds an item to the Buffer.
func (q *Buffer) Push(item interface{}) (ok bool) {
	q.lock()
	defer q.unlock()

	q.data = append(q.data, item)

	q.shift()
	return true
}

// Force will push the item onto the buffer and blocks until the operation completes.
func (q *Buffer) Force(item interface{}) {

	q.Push(item)
}

func (q *Buffer) shift() {
	if len(q.data) > 0 {
		select {
		case q.nextCh <- q.data[0]:
			q.data = q.data[1:]
		default:
		}
	}
}

// Pop returns then next value in the queue.
func (q *Buffer) Pop() (val interface{}, ok bool) {
	select {
	case item := <-q.nextCh:
		q.lock()
		q.shift()
		q.unlock()
		return item, true
	default:
		return nil, false
	}
}

// Pull pulls an item off the circular buffer. It blocks until a value is found.
func (q *Buffer) Pull() interface{} {
	for {
		v, ok := q.Pop()
		if !ok {
			runtime.Gosched()
			continue
		}
		return v
	}
}

func (q *Buffer) lock() {
	for {
		if atomic.CompareAndSwapInt32(&q.lockInt, unlocked, locked) {
			return
		}
		runtime.Gosched()
	}
}

func (q *Buffer) unlock() {
	for {
		if atomic.CompareAndSwapInt32(&q.lockInt, locked, unlocked) {
			return
		}
		runtime.Gosched()
	}
}
