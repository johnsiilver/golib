package ordered

import (
	"context"
	"testing"
)

func TestAdd(t *testing.T) {
	m := New[int, int]()

	for i := 0; i < 1000; i++ {
		m.Set(i, i)
	}

	for i := 0; i < 1000; i++ {
		v, ok := m.Get(i)
		if !ok {
			t.Errorf("missing key: %d", i)
		}
		if v != i {
			t.Errorf("wrong value: %d", v)
		}
	}

	i := 0
	for kv := range m.Range(context.Background()) {
		if kv.Key != i {
			t.Errorf("wrong key: %d", kv.Key)
		}
		if kv.Value != i {
			t.Errorf("wrong value: %d", kv.Value)
		}
		i++
	}
}

func TestDel(t *testing.T) {
	m := New[int, int]()

	for i := 0; i < 1000; i++ {
		m.Set(i, i)
	}

	if m.end.key != 999 {
		t.Errorf("wrong end key before delete: %d", m.end.key)
	}

	m.Del(999)
	if v, ok := m.Get(999); ok {
		t.Errorf("key not deleted: %d", v)
	}

	if m.end.key != 998 {
		t.Errorf("wrong end key after deletion: %d", m.end.key)
	}

	m.Del(0)
	if v, ok := m.Get(0); ok {
		t.Errorf("key not deleted: %d", v)
	}

	i := 1
	for v := range m.Range(context.Background()) {
		if v.Key != i {
			t.Errorf("wrong key: %d", v.Key)
		}
		if v.Value != i {
			t.Errorf("wrong value: %d", v.Value)
		}
		i++
	}
	if i != 999 { // Because we added 1 to i in the last iteration.
		t.Errorf("wrong number of keys after deleting first and last values: %d", i)
	}

	total := 998
	start := 1
	end := 998
	for total > 0 {
		m.Del(start)
		m.Del(end)
		total -= 2
		start++
		end--

		s := start
		var v KV[int, int]
		for v = range m.Range(context.Background()) {
			if v.Key != s {
				t.Errorf("wrong key: %d", v.Key)
			}
			if v.Value != s {
				t.Errorf("wrong value: %d", v.Value)
			}
			s++
		}
		if s != end+1 {
			t.Errorf("wrong number of keys after deleting first and last values: %d", s)
		}
	}

	if m.front != nil {
		t.Errorf("front not nil after deleting all keys")
	}
	if m.end != nil {
		t.Errorf("end not nil after deleting all keys")
	}
	if m.Len() != 0 {
		t.Errorf("wrong length after deleting all keys: %d", m.Len())
	}
}
