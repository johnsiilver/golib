package diskstack

import (
	"os"
	"path"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/pborman/uuid"
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
	err = d.Pop(&n)
	if err != nil {
		t.Fatalf("TestSimple: on retrieve %d: %v", 123, err)
	}

	if n != 123 {
		t.Fatalf("TestSimple: got %d, want 123", n)
	}
}

// TestTypes tests that all supported types work.
func TestTypes(t *testing.T) {
	str := "hello"
	integer := 2
	float := 2.2
	slice := []string{"hello"}
	maps := map[string]int{"yo": 1}
	structs := struct{ Name string }{Name: "john"}
	ptrStruct := &struct{ Name string }{Name: "john"}
	ptrBasic := new(int)

	tests := []struct {
		desc      string
		varType   interface{}
		reference bool
		ptr       bool
	}{
		{"string", str, false, false},
		{"intenger", integer, false, false},
		{"float", float, false, false},
		{"slice", slice, true, false},
		{"maps", maps, true, false},
		{"struct", structs, false, false},
		{"pointer to struct", ptrStruct, false, true},
		{"pointer to basic type", ptrBasic, false, true},
	}

	td := os.TempDir()

	for _, test := range tests {
		if test.reference || test.ptr {
			continue
		}

		p := path.Join(td, uuid.New())

		stack, err := New(p, test.varType, NoFlush())
		if err != nil {
			t.Fatalf("TestTypes: could not create stack: %s", err)
		}
		defer os.Remove(p)
		defer stack.Close()

		if err := stack.Push(test.varType); err != nil {
			t.Errorf("TestTypes(%s): could not push to stack: %s", test.desc, err)
			continue
		}

		v := reflect.New(reflect.TypeOf(test.varType))

		err = stack.Pop(v.Interface())
		if err != nil {
			t.Errorf("TestTypes(%s): could not pop from stack: %s", test.desc, err)
		}
	}
}

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
				err := d.Pop(&n)
				if err != nil {
					if err == StackEmpty {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					t.Fatalf("TestStack: on retrieve %d: %v", i, err)
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
