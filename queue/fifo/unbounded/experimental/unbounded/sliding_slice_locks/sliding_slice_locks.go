package sliding_slice_locks

import "sync"

// Buffer provides a first in first out non-blocking (but not lock free) Buffer.
type Buffer struct {
	mu     sync.Mutex
	nextCh chan interface{}
	data   []interface{}
}

// New is the contructor for Buffer.
func New() *Buffer {
	return &Buffer{nextCh: make(chan interface{}, 1)}
}

// Push adds an item to the Buffer.
func (q *Buffer) Push(item interface{}) {
	q.mu.Lock()
	q.data = append(q.data, item)
	q.shift()
	q.mu.Unlock()
}

// Force will push the item onto the buffer and blocks until the operation completes.
func (q *Buffer) Force(item interface{}) {
	q.Push(item)
}

// shift will move the next available entry from our queue into the channel to
// be returned to the user. Must be locked by a calling function.
func (q *Buffer) shift() {
	if len(q.data) > 0 {
		select {
		case q.nextCh <- q.data[0]:
			q.data = q.data[1:]
		default:
		}
	}
}

// Pop returns the next value in the buffer.  It returns ok == false if there
// is no value in the buffer.
func (q *Buffer) Pop() (val interface{}, ok bool) {
	select {
	case item := <-q.nextCh:
		q.mu.Lock()
		q.shift()
		q.mu.Unlock()
		return item, true
	default:
		return nil, false
	}
}

// Pull pulls an item off the circular buffer. It blocks until a value is found.
func (q *Buffer) Pull() interface{} {
	item := <-q.nextCh
	q.mu.Lock()
	q.shift()
	q.mu.Unlock()
	return item
}
