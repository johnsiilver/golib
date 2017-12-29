package bench

import (
	"fmt"
	"testing"

	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/custom_locks"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/custom_nosleep"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/custom_sleep_atomic"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/heap_atomic"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/heap_circular"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/heap_lock"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/reference"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/sliding_slice_atomic"
	"github.com/johnsiilver/golib/queue/fifo/unbounded/experimental/unbounded/sliding_slice_locks"
)

func BenchmarkUnboundedQueues(b *testing.B) {
	items := 100000
	runs := []struct{ items, senders, receivers int }{
		{items, 1, 1},
		{items, 10, 1},
		{items, 100, 1},
		{items, 10, 10},
		{items, 10, 100},
	}

	benchmarks := []struct {
		name    string
		newFunc func() unbounded
	}{

		{
			"reference",
			func() unbounded { return &reference.Buffer{Data: make(chan interface{}, 100)} },
		},

		{
			"sliding_slice_atomic",
			func() unbounded { return sliding_slice_atomic.New() },
		},
		{
			"sliding_slice_locks",
			func() unbounded { return sliding_slice_locks.New() },
		},
		{
			"custom_sleep_atomic",
			func() unbounded { return &custom_sleep_atomic.Buffer{} },
		},
		{
			"custom_nosleep",
			func() unbounded { return &custom_nosleep.Buffer{} },
		},
		{
			"custom_locks",
			func() unbounded { return &custom_locks.Buffer{} },
		},

		{
			"heap_atomic",
			func() unbounded { return &heap_atomic.Buffer{} },
		},

		{
			"heap_lock",
			func() unbounded { return &heap_lock.Buffer{} },
		},

		{
			"heap_circular",
			func() unbounded { return &heap_circular.Buffer{} },
		},
	}

	for _, run := range runs {
		for _, benchmark := range benchmarks {
			b.Run(
				benchmark.name+fmt.Sprintf("-items: %d, senders: %d, receivers: %d", run.items, run.senders, run.receivers),
				func(b *testing.B) {
					singleRun(b, benchmark.newFunc, run.items, run.senders, run.receivers)
				},
			)
		}
	}
}
