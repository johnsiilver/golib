package heap_lock

import (
	"sync"
	"testing"
)

type unbounded interface {
	Force(item interface{})
	Pull() interface{}
}

func BenchmarkRun(b *testing.B) {
	singleRun(b, func() unbounded { return &Buffer{} }, 100000, 10, 100)
}

func singleRun(bench *testing.B, n func() unbounded, items, senders, receivers int) {
	for i := 0; i < bench.N; i++ {
		bench.StopTimer()

		b := n()
		sendCh := make(chan int, items)
		wg := sync.WaitGroup{}
		wg.Add(items)

		// Setup senders.
		for i := 0; i < senders; i++ {
			go func() {
				for v := range sendCh {
					b.Force(v)
				}
			}()
		}

		// Setup receivers.
		for i := 0; i < receivers; i++ {
			go func() {
				for {
					b.Pull()
					wg.Done()
				}
			}()
		}

		bench.StartTimer()

		// Send to Buffer (which the receivers will read from)
		go func() {
			for i := 0; i < items; i++ {
				sendCh <- i
			}
			close(sendCh)
		}()

		wg.Wait()
	}
}
