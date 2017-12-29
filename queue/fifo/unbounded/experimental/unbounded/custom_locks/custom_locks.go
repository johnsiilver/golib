package custom_locks

import (
	"sync"
	"time"

	"github.com/johnsiilver/golib/queue/fifo/unbounded/internal/spin"
)

// Unbounded indicates that the Queue should have no memory bounds.
const Unbounded = -1

const (
	unlocked = int32(0)
	locked   = int32(1)
)

// Buffer provides a FIFO circular buffer that can grow and shrink as required.
// Buffer must not be copied after creation (which means use a pointer if
// passing between functions).
type Buffer struct {
	// Max is the maximum size the Buffer can grow to.  Use Unbounded if
	// you wish to grow the buffer to any size. By default this will grow to 1k items.
	Max int

	mu sync.Mutex

	lastShrink time.Time
	data       chan interface{}
}

// Push pushes an item onto the circular buffer. "ok" indicates if this happens.
func (c *Buffer) Push(item interface{}) (ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case c.data <- item:
		return true
	default:
	}

	c.grow()
	select {
	case c.data <- item:
		return true
	default:
		return false
	}
}

// Force will push the item onto the buffer and blocks until the operation completes.
func (c *Buffer) Force(item interface{}) {
	sleeper := spin.Sleeper{}

	for {
		if c.Push(item) {
			return
		}
		sleeper.Sleep()
	}
}

// Pop returns the next value off the circular buffer. If the buffer is empty
// ok will be false.
func (c *Buffer) Pop() (value interface{}, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case v := <-c.data:
		c.shrink()
		return v, true
	default:
		return nil, false
	}
}

// Pull pulls an item off the circular buffer. It blocks until a value is found.
func (c *Buffer) Pull() interface{} {
	sleeper := spin.Sleeper{}

	for {
		v, ok := c.Pop()
		if !ok {
			sleeper.Sleep()
			continue
		}
		return v
	}
}

// grow will double the size of the internal buffer until we hit the max size
// if the buffer is currently full.
// Note: grow must be protected with .lock/.unlock.
func (c *Buffer) grow() {
	if c.Max == 0 {
		c.Max = 1000
	}

	if cap(c.data) == c.Max {
		return
	}

	if len(c.data) == cap(c.data) {
		if c.Max == Unbounded {
			ch := make(chan interface{}, cap(c.data)*2)
			c.copy(ch, c.data)
			c.data = ch
			return
		}
		if cap(c.data) < c.Max {
			size := cap(c.data) * 2
			if size == 0 {
				size = 8
			}
			if size > c.Max {
				size = c.Max
			}
			ch := make(chan interface{}, size)
			c.copy(ch, c.data)
			c.data = ch
			return
		}
	}
}

// shrink shrinks the size of the internal buffer if the length of the buffer
// is < 50% of the capacity.  The reduction will be by 25%, but will produce
// a buffer size of no less than 8 slots.
// Note: shrink must be protected with .lock/.unlock.
func (c *Buffer) shrink() {
	if cap(c.data) == 8 {
		return
	}

	if time.Now().Sub(c.lastShrink) > 10*time.Minute {
		return
	}

	// If the current unused capacity is > 50% of the buffer, reduce it by 25%.
	if (cap(c.data) - len(c.data)) > (cap(c.data) / 2) {
		size := int(float64(cap(c.data)) * .75)
		if size < 8 {
			size = 8
		}

		ch := make(chan interface{}, size)
		c.copy(ch, c.data)
		c.data = ch
		c.lastShrink = time.Now()
	}
}

func (c *Buffer) copy(dst chan<- interface{}, src chan interface{}) {
	if (cap(dst) - len(dst)) < (cap(src) - len(src)) {
		panic("internal error: Buffer.copy() cannot be called when dst is smaller than src")
	}

	if src == nil {
		return
	}

	close(src)
	for v := range src {
		dst <- v
	}
}
