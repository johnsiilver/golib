package heap_circular

import (
	"sync"
	"time"

	"github.com/johnsiilver/golib/queue/fifo/unbounded/internal/spin"
)

type entry struct {
	v    interface{}
	next *entry
	prev *entry
}

type Buffer struct {
	wPtr, rPtr    *entry
	len, enqueued int
	mu            sync.Mutex
}

func (b *Buffer) Force(item interface{}) {
	b.mu.Lock()
	switch b.len {
	case 0:
		e := entry{v: item}
		e.next = &e
		e.prev = &e
		b.rPtr = &e
		b.wPtr = &e
		b.len++
	case 1:
		e := entry{v: item, next: b.wPtr, prev: b.wPtr}
		b.wPtr.next = &e
		b.wPtr.prev = &e
		b.wPtr = &e
		b.len++
	default:
		if b.wPtr.next == b.rPtr {
			e := entry{v: item, next: b.wPtr.next, prev: b.wPtr}
			b.wPtr.next = &e
			b.wPtr = &e
			b.len++
		} else { // reuse an older entry.
			b.wPtr.next.v = item
			b.wPtr = b.wPtr.next
		}
	}
	b.enqueued++
	b.mu.Unlock()
}

func (b *Buffer) Pull() interface{} {
	start := time.Now()
	sleeper := spin.Sleeper{}
	for {
		b.mu.Lock()
		switch {
		case b.enqueued == 0:
			break
		case b.enqueued == 1 && b.rPtr == b.wPtr:
			b.enqueued--
			v := b.rPtr.v
			if b.rPtr.next != nil {
				b.rPtr = b.rPtr.next
				b.wPtr = b.rPtr
			}
			b.mu.Unlock()
			return v
		case b.rPtr != b.wPtr:
			b.enqueued--
			v := b.rPtr.v
			b.rPtr = b.rPtr.next
			b.mu.Unlock()
			return v
		}

		b.mu.Unlock()
		sleeper.Sleep()
		if time.Now().Sub(start) > 5*time.Second {
			time.Sleep(5 * time.Second)
		}
	}
}
