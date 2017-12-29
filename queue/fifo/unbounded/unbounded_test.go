package unbounded

import (
	"fmt"
	"sync"
	"testing"
)

func TestPopPullBuffer(t *testing.T) {
	const size = 100000

	b := Buffer{}

	go func() {
		for i := 0; i < size; i++ {
			b.Push(i)
		}
	}()

	for i := 0; i < size; i++ {
		v := b.Pull().(int)
		if v != i {
			t.Errorf("TestFIFOBuffer: value at index %d was %d, should be %d", i, v, i)
		}
	}
}

func TestNext(t *testing.T) {
	const size = 100000

	b := Buffer{}

	for i := 0; i < size; i++ {
		b.Push(i)
	}
	b.Close()

	i := 0
	for v := range b.Next() {
		if v != i {
			t.Errorf("TestFIFOBuffer: value at index %d was %d, should be %d", i, v, i)
		}
		i++
	}
}

func BenchmarkUnbounded(b *testing.B) {
	items := 100000
	runs := []struct{ items, senders, receivers int }{
		{items, 1, 1},
		{items, 10, 1},
		{items, 100, 1},
		{items, 10, 10},
		{items, 10, 100},
	}

	for _, run := range runs {
		b.Run(
			fmt.Sprintf("BenchmarkUnboundedQueues-items: %d, senders: %d, receivers: %d", run.items, run.senders, run.receivers),
			func(b *testing.B) {
				singleRun(b, func() *Buffer { return &Buffer{} }, run.items, run.senders, run.receivers)
			},
		)
	}
}

func singleRun(bench *testing.B, n func() *Buffer, items, senders, receivers int) {
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
					b.Push(v)
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
		for i := 0; i < items; i++ {
			sendCh <- i
		}
		close(sendCh)

		wg.Wait()
	}
}
