package diskstack

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestStack(t *testing.T) {
	const (
		count = 1000
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
  if stat.Size() != 0 {
    t.Errorf("TestStack: file size should be 0 at the end, was: %v", stat.Size())
  }

  // Make sure our internal counters agree.
  if d.Size() != 0 {
    t.Errorf("TestStack: .Size(): got %d, want 0", d.Size())
  }

  if d.Len() != 0 {
    t.Errorf("TestStack: .Len(): got %d, want 0", d.Len())
  }
}
