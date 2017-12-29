package heap_lock

import (
	"sync"

	"github.com/johnsiilver/golib/queue/fifo/unbounded/internal/spin"
)

type queue struct {
	v    interface{}
	next *queue
}

type Buffer struct {
	ptr  *queue
	last *queue
	mu   sync.Mutex
}

func (b *Buffer) Force(item interface{}) {
	q := queue{v: item}

	b.mu.Lock()
	if b.ptr == nil {
		b.ptr = &q
		b.last = &q
	} else {
		b.last.next = &q
		b.last = &q
	}
	b.mu.Unlock()
}

func (b *Buffer) Pull() interface{} {
	sleeper := spin.Sleeper{}
	for {
		b.mu.Lock()
		if b.ptr != nil {
			v := b.ptr.v
			b.ptr = b.ptr.next
			b.mu.Unlock()
			return v
		}
		b.mu.Unlock()
		sleeper.Sleep()
	}
}
