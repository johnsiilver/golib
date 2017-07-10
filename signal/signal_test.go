package signal

import (
	"sort"
	"sync"
	"testing"

	"github.com/kylelemons/godebug/pretty"
)

func TestBufferSize(t *testing.T) {
	sig := New(BufferSize(5))
	for i := 0; i < 5; i++ {
		sig.Signal(i)
	}

	for i := 0; i < 5; i++ {
		ack := <-sig.Receive()
		if ack.Data().(int) != i {
			t.Errorf("TestBufferSize: loop %d: got %d, want %d", i, ack.Data().(int), i)
		}
		ack.Ack(nil)
	}
}

func TestPromise(t *testing.T) {
	sig := New()
	promises := make([]chan interface{}, 0, 100)
	want := make([]int, 0, 100)

	for i := 0; i < 100; i++ {
		i := i
		go func() {
			ack := <-sig.Receive()
			defer ack.Ack(i)
		}()
		p := make(chan interface{}, 1)
		promises = append(promises, p)
		want = append(want, i)
	}

	for i := 0; i < 100; i++ {
		sig.Signal(nil, Promise(promises[i]))
	}

	got := make([]int, 0, 100)
	for _, p := range promises {
		i := <-p
		got = append(got, i.(int))
	}

	sort.Ints(got)
	sort.Ints(want)
	if diff := pretty.Compare(want, got); diff != "" {
		t.Errorf("TestPromise: -want/+got:\n%s", diff)
	}
}

func TestClose(t *testing.T) {
	sig := New(BufferSize(3))
	wg := sync.WaitGroup{}
	wg.Add(1)
	got := []string{}
	go func() {
		defer wg.Done()
		for v := range sig.Receive() {
			v.Ack(nil)
			got = append(got, v.Data().(string))
		}
	}()

	sig.Signal("Hello")
	sig.Signal("World")
	sig.Close()
	wg.Wait()

	if diff := pretty.Compare([]string{"Hello", "World"}, got); diff != "" {
		t.Errorf("TestClose: -want/+got:\n%s", diff)
	}
}

func TestSignalWait(t *testing.T) {
	const hello = "hello everyone"
	const out = "I'm out!"

	var goReturn string
	var ackReturn string

	sig := New()

	go func(sig Signaler) {
		ack := <-sig.Receive()
		defer ack.Ack(out)

		goReturn = ack.Data().(string)
	}(sig)

	ackReturn = sig.Signal(hello, Wait()).(string)

	if goReturn != hello {
		t.Errorf("TestSignalWait: got goReturn == %s, want %s", goReturn, hello)
	}
	if ackReturn != out {
		t.Errorf("TestSignalWait: got ackReturn == %s, want %s", ackReturn, out)
	}
}
