package heap_atomic

import (
	"runtime"
	"sync/atomic"

	"github.com/johnsiilver/golib/queue/fifo/unbounded/internal/spin"
)

// Unbounded indicates that the Queue should have no memory bounds.
const Unbounded = -1

const (
	unlocked = int32(0)
	locked   = int32(1)
)

type queue struct {
	v    interface{}
	next *queue
}

type Buffer struct {
	ptr     *queue
	last    *queue
	lockInt int32
}

func (b *Buffer) Force(item interface{}) {
	b.lock()
	if b.ptr == nil {
		q := queue{v: item}
		b.ptr = &q
		b.last = &q
	} else {
		q := queue{v: item}
		b.last.next = &q
		b.last = &q
	}
	b.unlock()
}

func (b *Buffer) Pull() interface{} {
	sleeper := spin.Sleeper{}
	for {
		b.lock()
		if b.ptr != nil {
			v := b.ptr.v
			b.ptr = b.ptr.next
			b.unlock()
			return v
		}
		b.unlock()
		sleeper.Sleep()
	}
}

func (b *Buffer) lock() {
	for {
		if atomic.CompareAndSwapInt32(&b.lockInt, unlocked, locked) {
			return
		}
		runtime.Gosched()
	}
}

func (b *Buffer) unlock() {
	for {
		if atomic.CompareAndSwapInt32(&b.lockInt, locked, unlocked) {
			return
		}
		runtime.Gosched()
	}
}
