package diskstack

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/beeker1121/goque"
)

func TestSimple(t *testing.T) {
	const p = "./stack_tmp"

	// Remove the test file if it somehow exists.
	_, err := os.Stat(p)
	if err == nil {
		if err := os.Remove(p); err != nil {
			t.Fatalf("TestStack: test file exists and could not be deleted: %s", err)
		}
	}

	// Remove the test file before closing.
	defer os.Remove(p)

	d, err := New(p, int(0))
	if err != nil {
		t.Fatalf("TestSimple: %s", err)
	}
	defer d.Close()

	if err := d.Push(int(123)); err != nil {
		t.Fatalf("TestSimple: on entry %d: %v", 123, err)
	}

	var n int
	ok, err := d.Pop(&n)
	if err != nil {
		t.Fatalf("TestSimple: on retrieve %d: %v", 123, err)
	}

	if !ok {
		t.Fatalf("TestSimple: !ok returned")
	}

	if n != 123 {
		t.Fatalf("TestSimple: got %d, want 123", n)
	}
}

func TestStack(t *testing.T) {
	const (
		count = 100000
		p     = "./stack_tmp"
	)

	// Remove the test file if it somehow exists.
	_, err := os.Stat(p)
	if err == nil {
		if err := os.Remove(p); err != nil {
			t.Fatalf("TestStack: test file exists and could not be deleted: %s", err)
		}
	}

	// Remove the test file before closing.
	defer os.Remove(p)

	d, err := New(p, int(0))
	if err != nil {
		t.Fatalf("TestStack: %s", err)
	}
	defer d.Close()

	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("TestStack: could not stat the file: %s", err)
	}
	sizeWithHeader := fi.Size()

	// Push count integers onto the stack and read count integers off the stack
	// and mark it received into a []bool.
	wg := sync.WaitGroup{}
	wg.Add(count)
	v := make([]bool, count)
	for i := 0; i < count; i++ {
		go func(i int) {
			if err := d.Push(i); err != nil {
				t.Fatalf("TestStack: on entry %d: %v", i, err)
			}
		}(i)

		go func() {
			defer wg.Done()
			var n int

			for {
				ok, err := d.Pop(&n)
				if err != nil {
					t.Fatalf("TestStack: on retrieve %d: %v", i, err)
				}

				if !ok {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				v[n] = true
				return
			}
		}()
	}

	wg.Wait()

	// Validate that all integers were found.
	for i, ok := range v {
		if !ok {
			t.Errorf("TestStack: entry %d was not seen", i)
		}
	}

	// Make sure the size of the file was 0.
	stat, err := os.Stat(p)
	if err != nil {
		t.Errorf("TestStack: could not stat %s: %s", p, err)
	}
	if stat.Size() != sizeWithHeader {
		t.Errorf("TestStack: file size should be 0 at the end, was: %v", stat.Size())
	}

	// Make sure our internal counters agree.
	if d.Size() != int(sizeWithHeader) {
		t.Errorf("TestStack: .Size(): got %d, want 0", d.Size())
	}

	if d.Len() != 0 {
		t.Errorf("TestStack: .Len(): got %d, want 0", d.Len())
	}
}

func TestGoque(t *testing.T) {
	const count = 100000

	s, err := goque.OpenStack("./data_dir")
	if err != nil {
		panic(err)
	}
	//defer s.Drop()
	defer s.Close()

	wg := sync.WaitGroup{}
	wg.Add(count)
	v := make([]bool, count)
	for i := 0; i < count; i++ {
		go func(i int) {
			if _, err := s.PushObject(i); err != nil {
				t.Fatalf("TestStack: on entry %d: %v", i, err)
			}
		}(i)

		go func() {
			defer wg.Done()
			var n int

			for {
				item, err := s.Pop()
				if err != nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}
        err = item.ToObject(&n)
        if err != nil {
          panic(err)
        }
				v[n] = true
				return
			}
		}()
	}

	wg.Wait()

	// Validate that all integers were found.
	for i, ok := range v {
		if !ok {
			t.Errorf("TestStack: entry %d was not seen", i)
		}
	}
}
