package boutique

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
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
	glog.Infof("TestCopyAndAppendSlice")
	defer glog.Infof("End TestCopyAndAppendSlice")

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
	glog.Infof("TestFieldsChanged")
	defer glog.Infof("End TestFieldsChanged")

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
	glog.Infof("TestSubscribe")
	defer glog.Infof("End TestSubscribe")

	glog.Infof("--subscribeSignalsCorrectly")
	subscribeSignalsCorrectly(t)
	glog.Infof("--end subscribeSignalsCorrectly")
	glog.Infof("--signalsDontBlock")
	signalsDontBlock(t)
	glog.Infof("--end signalsDontBlock")
	glog.Infof("--cancelWorks")
	cancelWorks(t)
	glog.Infof("--end cancelWorks")
}

func subscribeSignalsCorrectly(t *testing.T) {
	initial := MyState{
		Counter: 0,
		Status:  "",
		List:    []string{},
		Dict:    map[string]bool{},
	}
	freeze.Object(&initial)

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList), nil)
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	sch, _, err := s.Subscribe("Status")
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}
	lch, _, err := s.Subscribe("List")
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}
	ach, _, err := s.Subscribe(Any)
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	_, _, err = s.Subscribe("private")
	if err == nil {
		t.Errorf("TestPerform: s.Subcribe(\"private\"): err == nil, want error != nil")
	}

	mu := &sync.Mutex{}
	count := counters{}
	countDone := make(chan Signal, 1)
	go func() {
		for {
			select {
			case s := <-sch:
				if s.Fields[0] != "Status" {
					t.Fatalf("Test subscribeSignalsCorrectly: Signal for field Status had FieldsChanged set to: %s", s.Fields[0])
				}
				mu.Lock()
				count.status++
				mu.Unlock()
			case s := <-lch:
				if s.Fields[0] != "List" {
					t.Fatalf("Test subscribeSignalsCorrectly: Signal for field List had FieldsChanged set to: %s", s.Fields[0])
				}
				mu.Lock()
				count.list++
				mu.Unlock()
			case <-ach:
				mu.Lock()
				count.any++
				mu.Unlock()
			}
			countDone <- Signal{}
		}
	}()

	status := "status"
	loops := 1000
	for i := 0; i < loops; i++ {
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

	want := counters{status: loops, list: loops / 2, any: loops + (loops / 2)}
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

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList), nil)
	if err != nil {
		t.Fatalf("signalsDontBlock: %s", err)
	}

	ch, _, err := s.Subscribe("Counter")
	if err != nil {
		t.Fatalf("signalsDontBlock: %s", err)
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

func cancelWorks(t *testing.T) {
	initial := MyState{Counter: 0}

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList), nil)
	if err != nil {
		t.Fatalf("cancelWorks: %s", err)
	}

	tests := []struct {
		desc          string
		subscriptions int
		keyExist      bool
	}{
		{
			desc:          "1 subscriber",
			subscriptions: 1,
			keyExist:      false,
		},
		{
			desc:          "2 subscribers",
			subscriptions: 2,
			keyExist:      true,
		},
	}

	for _, test := range tests {
		var (
			ch     chan Signal
			cancel CancelFunc
			err    error
		)

		for i := 0; i < test.subscriptions; i++ {
			// We only want to keep the last set of these.
			ch, cancel, err = s.Subscribe("Counter")
			if err != nil {
				t.Fatalf("cancelWorks: test %s: %s", test.desc, err)
			}
		}

		s.Perform(IncrCounter(), nil)

		sawBroadcast := make(chan Signal, 10)
		chClosed := make(chan Signal)
		go func() {
			defer close(chClosed)
			for _ = range ch {
				sawBroadcast <- Signal{}
			}
		}()

		select {
		case <-sawBroadcast:
		case <-time.After(5 * time.Second):
			t.Fatalf("cancelWorks: test %s: did not see anything on the subscription channel", test.desc)
		}

		cancel()

		s.Perform(IncrCounter(), nil)

		select {
		case <-chClosed:
		case <-time.After(5 * time.Second):
			t.Fatalf("cancelWorks: test %s: did not see the channel close", test.desc)
		}

		_, ok := s.subscribers["Counter"]
		if test.keyExist && !ok {
			t.Fatalf("cancelWorks, test %s: expected to see the key in subscribers, but didn't", test.desc)
		}
		if !test.keyExist && ok {
			t.Fatalf("cancelWorks, test %s: expected to not find the key in subscribers, but did", test.desc)
		}
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

	s, err := New(initial, NewModifier(UpCounter, UpStatus, UpList), nil)
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	wg := &sync.WaitGroup{}
	status := ""
	list := []string{}
	loop := 1000
	for i := 0; i < loop; i++ {
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
		s.State().Data.(MyState),
	)

	if s.State().Version != uint64(loop*3) {
		t.Errorf("Test TestPerform: got Version == %d, want %d", s.State().Version, loop*3)
	}

	switch {
	case s.State().FieldVersions["Status"] != uint64(loop):
		t.Errorf("Test TestPerform: got FieldVersion['Status'] == %d, want %d", s.State().FieldVersions["Status"], loop)
	case s.State().FieldVersions["Counter"] != uint64(loop):
		t.Errorf("Test TestPerform: got FieldVersion['Counter'] == %d, want %d", s.State().FieldVersions["Counter"], loop)
	case s.State().FieldVersions["List"] != uint64(loop):
		t.Errorf("Test TestPerform: got FieldVersion['List'] == %d, want %d", s.State().FieldVersions["List"], loop)
	}

	if diff != "" {
		t.Errorf("Test TestPerform: -want/+got:\n%s", diff)
	}
}

func TestMiddleware(t *testing.T) {
	initial := MyState{
		Counter: -1,
	}

	logs := []int{}
	logger := func(a Action, newData interface{}, getState GetState, committed chan State, wg *sync.WaitGroup) (changedData interface{}, stop bool, err error) {
		go func() {
			defer wg.Done()
			state := <-committed
			if state.IsZero() {
				return
			}
			logs = append(logs, state.Data.(MyState).Counter)
		}()
		return nil, false, nil
	}

	fiveHundredToOneThousand := func(a Action, newData interface{}, getState GetState, committed chan State, wg *sync.WaitGroup) (changedData interface{}, stop bool, err error) {
		defer wg.Done()
		data := newData.(MyState)
		if data.Counter == 500 {
			data.Counter = 1000
		}
		return data, false, nil
	}

	skipSevenHundred := func(a Action, newData interface{}, getState GetState, committed chan State, wg *sync.WaitGroup) (changedData interface{}, stop bool, err error) {
		defer wg.Done()
		data := newData.(MyState)
		if data.Counter == 700 {
			return nil, true, nil
		}
		return nil, false, nil
	}

	sawSevenHundred := false
	sevenHundred := func(a Action, newData interface{}, getState GetState, committed chan State, wg *sync.WaitGroup) (changedData interface{}, stop bool, err error) {
		defer wg.Done()
		data := newData.(MyState)
		if data.Counter == 700 {
			sawSevenHundred = true
		}
		return nil, false, nil
	}

	errorEightHundred := func(a Action, newData interface{}, getState GetState, committed chan State, wg *sync.WaitGroup) (changedData interface{}, stop bool, err error) {
		defer wg.Done()
		data := newData.(MyState)
		if data.Counter == 800 {
			return nil, false, fmt.Errorf("error")
		}
		return nil, false, nil
	}

	middle := []Middleware{logger, fiveHundredToOneThousand, skipSevenHundred, sevenHundred, errorEightHundred}

	s, err := New(initial, NewModifier(UpCounter), middle)
	if err != nil {
		t.Fatalf("TestPerform: %s", err)
	}

	for i := 0; i < 1000; i++ {
		s.Perform(IncrCounter(), nil)
	}

	wantLogs := []int{}
	counter := 0
	for i := 0; i < 1000; i++ {
		switch counter {
		case 500:
			wantLogs = append(wantLogs, 1000)
			counter = 1001
		case 800:
			continue
		default:
			wantLogs = append(wantLogs, counter)
			counter++
		}
	}

	if sawSevenHundred == true {
		t.Errorf("Test TestMiddleware: middleware sevenHundred was called on update of 700, which should have been prevented by skipSevenHundred")
	}

	if diff := pretty.Compare(logs, wantLogs); diff != "" {
		t.Errorf("Test TestMiddleware: -want/+got:\n%s", diff)
	}
}
