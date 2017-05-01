package boutique

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/lukechampine/freeze"
)

const (
	unknown = iota
	incrCounter
	decrCounter
	appendList
	statusChange
)

type MyState struct {
	Counter int
	Status  string
	List    []string
	Dict    map[string]bool
	private bool
}

// Actions

func IncrCounter() Action {
	return Action{
		Type: incrCounter,
	}
}

func DecrCounter() Action {
	return Action{
		Type: decrCounter,
	}
}

func Status(s string) Action {
	return Action{
		Type:   statusChange,
		Update: s,
	}
}

// ChangeList replaces the items in the list with l.
func AppendList(item string) Action {
	return Action{
		Type:   appendList,
		Update: item,
	}
}

// Updaters

func UpCounter(state interface{}, action Action) interface{} {
	s := state.(MyState)
	switch action.Type {
	case incrCounter:
		s.Counter++
	case decrCounter:
		s.Counter--
	}
	return s
}

func UpStatus(state interface{}, action Action) interface{} {
	s := state.(MyState)
	switch action.Type {
	case statusChange:
		s.Status = action.Update.(string)
	}
	return s
}

func UpList(state interface{}, action Action) interface{} {
	s := state.(MyState)

	switch action.Type {
	case appendList:
		nl := CopyAppendSlice(s.List, action.Update.(string)).([]string)
		s.List = nl
	}
	return s
}

func TestModifier(t *testing.T) {
	m := NewModifier(UpCounter, UpStatus)
	s := MyState{}

	v := m.run(s, IncrCounter())
	if v.(MyState).Counter != 1 {
		t.Errorf("Test TestModifier: Counter: got %d, want %d", v.(MyState).Counter, 1)
	}
	v = m.run(s, IncrCounter())
	if v.(MyState).Counter != 1 {
		t.Errorf("Test TestModifier: Counter: got %d, want %d", v.(MyState).Counter, 1)
	}
	v = m.run(s, DecrCounter())
	if v.(MyState).Counter != -1 {
		t.Errorf("Test TestModifier: Counter: got %d, want %d", v.(MyState).Counter, -1)
	}
	v = m.run(s, Status("Hello World"))
	if v.(MyState).Status != "Hello World" {
		t.Errorf("Test TestModifier: Status: got %s, want %s", v.(MyState).Status, "Hello World")
	}
}

func TestCopyAndAppendSlice(t *testing.T) {
	a := "apples"
	b := "oranges"

	tests := []struct {
		desc  string
		slice interface{}
		item  interface{}
		want  interface{}
		err   bool
	}{
		{
			desc:  "Error: 'slice' arg is not a slice",
			slice: 3,
			item:  2,
			err:   true,
		},
		{
			desc:  "Error: 'item' arg is not same type as slice",
			slice: []string{"apples"},
			item:  2,
			err:   true,
		},
		{
			desc:  "Success with []string",
			slice: []string{"apples"},
			item:  "oranges",
			want:  []string{"apples", "oranges"},
		},
		{
			desc:  "Success with []*string",
			slice: []*string{&a},
			item:  &b,
			want:  []*string{&a, &b},
		},
	}

	conf := pretty.Config{
		IncludeUnexported: false,
		Diffable:          true,
	}

	for _, test := range tests {
		// Make sure we don't modify the starting slice.
		if reflect.TypeOf(test.slice).Kind() == reflect.Slice {
			freeze.Slice(test.slice)
		}

		got, err := copyAppendSlice(test.slice, test.item)

		switch {
		case err == nil && test.err:
			t.Errorf("Test %s: got err == nil, want err != nil", test.desc)
			continue
		case err != nil && !test.err:
			t.Errorf("Test %s: got err != %s, want err == nil", test.desc, err)
			continue
		case err != nil:
			continue
		}

		if diff := conf.Compare(test.want, got); diff != "" {
			t.Errorf("Test %s: -want/+got:\n%s", test.desc, diff)
		}
	}
}

func TestFieldsChanged(t *testing.T) {
	os := MyState{Dict: map[string]bool{}}
	ns := MyState{Counter: 1, Dict: map[string]bool{}}

	changed := fieldsChanged(os, ns)
	if diff := pretty.Compare([]string{"Counter"}, changed); diff != "" {
		t.Errorf("TestFieldsChanged: -want/+got:\n%s", diff)
	}

	ns.List = append(ns.List, "hello")
	changed = fieldsChanged(os, ns)
	if diff := pretty.Compare([]string{"Counter", "List"}, changed); diff != "" {
		t.Errorf("TestFieldsChanged: -want/+got:\n%s", diff)
	}
	ns.Dict["key"] = true
	changed = fieldsChanged(os, ns)
	if diff := pretty.Compare([]string{"Counter", "List", "Dict"}, changed); diff != "" {
		t.Errorf("TestFieldsChanged: -want/+got:\n%s", diff)
	}

	// changed should not alter because ns.private is a a private variable.
	ns.private = true
	changed = fieldsChanged(os, ns)
	if diff := pretty.Compare([]string{"Counter", "List", "Dict"}, changed); diff != "" {
		t.Errorf("TestFieldsChanged: -want/+got:\n%s", diff)
	}
}

type counters struct {
	counter, status, list, dict, any int
}

func TestSubscribe(t *testing.T) {
	subscribeSignalsCorrectly(t)
	signalsDontBlock(t)
}

func subscribeSignalsCorrectly(t *testing.T) {
	initial := MyState{
		Counter: 0,
		Status:  "",
		List:    []string{},
		Dict:    map[string]bool{},
	}
	freeze.Object(&initial)

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList))
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	sch, err := s.Subscribe("Status")
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}
	lch, err := s.Subscribe("List")
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}
	ach, err := s.Subscribe(Any)
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	_, err = s.Subscribe("private")
	if err == nil {
		t.Errorf("TestPerform: s.Subcribe(\"private\"): err == nil, want error != nil")
	}

	mu := &sync.Mutex{}
	count := counters{}
	countDone := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-sch:
				mu.Lock()
				count.status++
				mu.Unlock()
			case <-lch:
				mu.Lock()
				count.list++
				mu.Unlock()
			case <-ach:
				mu.Lock()
				count.any++
				mu.Unlock()
			}
			countDone <- struct{}{}
		}
	}()

	status := "status"
	for i := 0; i < 1000; i++ {
		status = status + ":changed:"
		s.Perform(Status(status), nil)
		<-countDone
		<-countDone // For Any

		if i%2 == 0 {
			s.Perform(AppendList("a"), nil)
			<-countDone
			<-countDone // For Any
		}

	}

	want := counters{status: 1000, list: 500, any: 1500}
	if diff := pretty.Compare(want, count); diff != "" {
		t.Errorf("TestSubscribe: -want/+got:\n%s", diff)
	}
}

func signalsDontBlock(t *testing.T) {
	// NOTE: initial must fully be filled out or freeze will nil pointer dereference.
	initial := MyState{
		Counter: 0,
		Status:  "",
		List:    []string{},
		Dict:    map[string]bool{},
	}
	freeze.Object(&initial)

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList))
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	ch, err := s.Subscribe("Counter")
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	for i := 0; i < 100; i++ {
		s.Perform(IncrCounter(), nil)
	}
	// time.Sleep() things are prone to break, but I don't want to have to put a
	// signaling mechanism to know when the go cast() of finished.
	time.Sleep(1 * time.Second)
	<-ch
	select {
	case <-ch:
		t.Errorf("signalsDontBlock: got <-ch had something on it, want <-ch to block")
	default:
	}
}

func TestPerform(t *testing.T) {
	initial := MyState{
		Counter: 0,
		Status:  "",
		List:    []string{},
		Dict:    map[string]bool{},
	}
	freeze.Object(&initial)

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList))
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	wg := &sync.WaitGroup{}
	status := ""
	list := []string{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		status = status + "a"
		s.Perform(Status(status), wg) // Cannot be done in a goroutine.

		wg.Add(1)
		go s.Perform(IncrCounter(), wg)

		wg.Add(1)
		list = append(list, "a")
		go s.Perform(AppendList("a"), wg)
	}

	wg.Wait()

	diff := pretty.Compare(
		MyState{
			Counter: 1000,
			Status:  status,
			List:    list,
		},
		s.State().(MyState),
	)
	if diff != "" {
		t.Errorf("Test TestPerform: -want/+got:\n%s", diff)
	}
}
