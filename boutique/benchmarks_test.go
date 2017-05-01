package boutique

import (
	"sync"
	"testing"
)

func BenchmarkConcurrentPerform(b *testing.B) {
	// About .519383694s to do 100k operations.
	// 6.8 times slower than BenchmarkStaticallyWithMutation
	// 5.8 times slower than BenchmarkStaticallyWithoutMutation

	// At 500 operations, these both appear to be about 7 times faster.

	initial := MyState{Counter: 0}

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList))
	if err != nil {
		b.Fatalf("BenchmarkConcurrentPerform: %s", err)
	}
	wg := &sync.WaitGroup{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := 0; i < 500; i++ {
			wg.Add(1)
			go s.Perform(IncrCounter(), wg)
		}
		wg.Wait()
	}
}

func BenchmarkStaticallyWithMutation(b *testing.B) {
	// About 0.075974059s to do 100k operations
	initial := &MyState{Counter: 0}
	mu := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	for i := 0; i < b.N; i++ {
		for i := 0; i < 500; i++ {
			wg.Add(1)
			go func() {
				mu.Lock()
				defer mu.Unlock()
				defer wg.Done()
				initial.Counter++
			}()
		}
		wg.Wait()
	}
}

func BenchmarkStaticallyWithoutMutation(b *testing.B) {
	// About 0.088297825 to do 100k operations

	// NOTE: If you make changes here, make sure to use freeze.Object() on MyState
	// before submitting.  Remove freeze afterwards.

	initial := MyState{Counter: 0}

	mu := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	sc := func(ms MyState) MyState {
		return ms
	}

	for i := 0; i < b.N; i++ {
		for i := 0; i < 500; i++ {
			wg.Add(1)
			go func() {
				mu.Lock()
				defer mu.Unlock()
				defer wg.Done()
				ms := sc(initial)
				ms.Counter++
				initial = ms
			}()
		}
		wg.Wait()
	}
}
