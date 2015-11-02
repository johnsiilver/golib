package lru

import (
  "strings"
  "testing"

  "github.com/kr/pretty"
)

func TestLRU(t *testing.T) {
  const sizeLimit = 3

  c, err := New(NumberLimit(sizeLimit), PreSize(sizeLimit))
  if err != nil {
    t.Fatalf("got err == %q, want err == nil", err)
  }

  // Add "sizeLimit" entries to the cache.
  for i := 0; i < sizeLimit; i++ {
    if err := c.Set(i, i); err != nil {
      t.Fatalf("got err == %q, want err == nil", err)
    }
  }

  // Get all values that should be stored. Because we get all the values, we should then
  // be ordered in the same least recently used order as we started with.
  for i := 0; i < sizeLimit; i++ {
    v, ok := c.Get(i)
    if !ok {
      t.Fatalf("c.Get(%d) returned false, which should not happen", i)
    }

    if v.(int) != i {
      t.Fatalf("c.Get(%d) returned %d, want %d", v.(int), i)
    }
  }

  // Add a value that should push the 0 value out to add a new entry.
  if err := c.Set(sizeLimit, sizeLimit); err != nil {
    t.Fatalf("got err == %q, want err == nil", err)
  }

  // Now get the newest value.
  _, ok := c.Get(sizeLimit)
  if !ok {
    t.Fatalf("c.Get(sizeLimit) returned false, which should not happen" )
  }

  // Make sure the 0 value has been removed.
  _, ok = c.Get(0)
  if ok {
    t.Fatalf("c.Get(0) returned true, but 0 should have been removed from the cache")
  }

  // Verify our cache has not gotten bigger than our size limit.
  r := c.(*cache)
  if len(r.cache) != sizeLimit{
    t.Fatalf("internal map size: got %d, want %d", len(r.cache), sizeLimit)
  }

  // Check out linked list to make sure it has the right values.
  vals := retrieveList(r.startList, r.endList, t)

  if diff := pretty.Diff(vals, []int{1, 2, 3}); len(diff) != 0 {
    t.Errorf("got: %#v", vals)
    t.Errorf("want: %#v", []int{1, 2, 3})
    t.Fatalf("internal nodes: got/want diff:\n%s", strings.Join(diff, "\n"))
  }

  // Make sure a get moves the value to the end.
  _, ok = c.Get(2)
  if !ok {
    t.Fatalf("could not find value 2")
  }

  vals = retrieveList(r.startList, r.endList, t)

  if diff := pretty.Diff(vals, []int{1, 3, 2}); len(diff) != 0 {
    t.Errorf("got: %#v", vals)
    t.Errorf("want: %#v", []int{1, 2, 3})
    t.Fatalf("internal nodes: got/want diff:\n%s", strings.Join(diff, "\n"))
  }
}

func TestRemove(t *testing.T) {
  c, err := New(NumberLimit(3), PreSize(3))
  if err != nil {
    t.Fatalf("got err == %q, want err == nil", err)
  }

  tests := []struct{
    desc string
    change func()
    vals []int
  }{
    {
      desc: "Remove key 2 which doesn't exist",
      change: func(){c.Remove(2)},
      vals: []int{},
    },
    {
      desc: "Add keys 0, 1, 2",
      change: func() {
        for i := 0; i < 3; i++{
          c.Set(i, i)
        }
      },
      vals: []int{0, 1, 2},
    },
    {
      desc: "Remove the middle node, key 1",
      change: func(){c.Remove(1)},
      vals: []int{0, 2},
    },
    {
      desc: "Add new node, key 3",
      change: func(){c.Set(3, 3)},
      vals: []int{0, 2, 3},
    },
    {
      desc: "Remove the last node, key 3",
      change: func(){c.Remove(3)},
      vals: []int{0, 2},
    },
    {
      desc: "Remove the first node key 0",
      change: func(){c.Remove(0)},
      vals: []int{2},
    },
    {
      desc: "Remove the root node, key 2",
      change: func(){c.Remove(2)},
      vals: []int{},
    },
  }

  r := c.(*cache)
  for _, test := range tests{
    test.change()

    vals := retrieveList(r.startList, r.endList, t)
    if diff := pretty.Diff(vals, test.vals); len(diff) != 0 {
     t.Fatalf("internal nodes: got/want diff:\n%s", strings.Join(diff, "\n"))
    }

  }
}

func retrieveList(startList, endList *node, t *testing.T) []int {
  ptr := startList
  vals := []int{}

  if ptr == nil {
    return []int{}
  }

  for {
      if ptr == nil {
        return vals
      }

      if ptr.prev == nil && ptr != startList {
        t.Errorf("node at key %v has .prev == nil, but is not the start of the list", ptr.k)
      }
      if ptr.next == nil && ptr != endList {
        t.Errorf("node at key %v has .next == nil, but is not the end of the list", ptr.k)
      }

      vals = append(vals, ptr.v.(int))
      ptr = ptr.next
  }
  return vals
}
