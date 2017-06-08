package boutique

import (
	"log"
	"sync"
	"testing"

	"net/http"
	_ "net/http/pprof"
)

/*
BenchmarkConcurrentPerform-8                     	    3000	    542119 ns/op
BenchmarkMultifieldPerform-8                     	     500	   3387714 ns/op
BenchmarkStaticallyWithMutation-8                	    5000	    295028 ns/op
BenchmarkStaticallyWithoutMutation-8             	    5000	    300650 ns/op
BenchmarkStaticallyMultifieldWithoutMutation-8   	    1000	   1719393 ns/op

Single field changing with a big lock:

Perform is 1.8 times slower than a version using mutation
Perform is 1.8 times slower than a version doing copies

If we move up to changing multiple fields simultaneously with a big lock, this becomes:

Perform is 1.9 times slower than a version doing copies

These benchmarks don't take into account the biggest cost in using this module,
which is calculating the changes for subscribers.
*/

func init() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
}

const iterations = 500

func BenchmarkConcurrentPerform(b *testing.B) {
	initial := MyState{Counter: 0}

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList), nil)
	if err != nil {
		b.Fatalf("BenchmarkConcurrentPerform: %s", err)
	}
	wg := &sync.WaitGroup{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := 0; i < iterations; i++ {
			wg.Add(1)
			go s.Perform(IncrCounter(), wg)
		}
		wg.Wait()
	}
}

func BenchmarkMultifieldPerform(b *testing.B) {
	initial := MyState{Counter: 0, List: []string{}}

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList), nil)
	if err != nil {
		b.Fatalf("BenchmarkMultifieldConcurrentPerform: %s", err)
	}
	wg := &sync.WaitGroup{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := 0; i < iterations; i++ {
			wg.Add(1)
			//go s.Perform(IncrCounter(), wg)
			go s.Perform(AppendList("a"), wg)
		}
		wg.Wait()

		// We have to reset the store, otherwise the List will just keep growning on
		// each test and the benchamrk will not stop.
		b.StopTimer()
		s.state.Store(MyState{Counter: 0, List: []string{}})
		b.StartTimer()
	}
}

func BenchmarkStaticallyWithMutation(b *testing.B) {
	initial := &MyState{Counter: 0}
	mu := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	for i := 0; i < b.N; i++ {
		for i := 0; i < iterations; i++ {
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
	// NOTE: If you make changes here, make sure to use freeze.Object() on MyState
	// before submitting.  Remove freeze afterwards.

	initial := MyState{Counter: 0}

	mu := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	sc := func(ms MyState) MyState {
		return ms
	}

	for i := 0; i < b.N; i++ {
		for i := 0; i < iterations; i++ {
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

func BenchmarkStaticallyMultifieldWithoutMutation(b *testing.B) {
	initial := MyState{Counter: 0, List: []string{}}

	mu := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	sc := func(ms MyState) MyState {
		return ms
	}

	copyAppendSlice := func(s []string, item string) []string {
		cp := make([]string, len(s)+1)
		copy(cp, s)
		cp[len(cp)-1] = item
		return cp
	}

	for i := 0; i < b.N; i++ {
		for i := 0; i < iterations; i++ {
			wg.Add(2)
			go func() {
				mu.Lock()
				defer mu.Unlock()
				defer wg.Done()
				ms := sc(initial)
				ms.Counter++
				initial = ms
			}()
			go func() {
				mu.Lock()
				defer mu.Unlock()
				defer wg.Done()
				ms := sc(initial)
				ms.List = copyAppendSlice(ms.List, "a")
				initial = ms
			}()
		}
		wg.Wait()

		// We have to reset the store, otherwise the List will just keep growning on
		// each test and the benchamrk will not stop.
		b.StopTimer()
		initial = MyState{Counter: 0, List: []string{}}
		b.StartTimer()
	}
}
