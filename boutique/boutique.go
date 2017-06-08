/*
Package boutique provides an immutable state storage with subscriptions to
changes in the store. It is intended to be a single storage solution for a
service.  The design has its origins in Flux and Redux.

Features and Drawbacks

Features:

 * Immutable data does not require locking outside the store.
 * Subscribing to individual field changes are simple.
 * Data locking is handled by the Container.

Drawbacks:

 * You are giving up static type checks on compile.
 * Internal reflection adds about 1.8x overhead.
 * Must be careful to not mutate data.

Immutability

When we say immutable, we mean that everything gets copied, as Go
does not have immutable objects or types other than strings. This means every
update to a pointer or reference type (map, dict, slice) must make a copy of the
data before changing it, not a mutation.  Because of modern processors, this
copying is quite fast.

Usage structure

Boutique provides storage that is best designed in a modular method:

  └── storage
    ├── storage.go
    ├── actions
    │   └── actions.go
    └── updaters
        └── updaters.go

The files are best organized by using them as follows:

  storage.go - Holds the store object
  actions.go - Holds the actions that will be used by the updaters
  updaters.go - Holds all the actions that change the store's data


  Note: These are all simply suggestions, you can combine this in a single file or name the files whatever you wish.

Example

Below is an exmple of an application state stored in Boutique.

Design your state object

The state object holds all of your state. The only rule for a state object is that
it must be a struct (not to be confused with a *struct).

  // MyState is a state object for a fictitious service. We are tracking
  // connections to other services, the application's state, when we completed
  // our last action, and how many actions are currently being run.
  // Note: you can only receive updates on a field change if the field is
  // public.
  type MyState struct {
    // Connections holds a mapping of names to remote GRPC connections.
    Connections: map[string]*grpc.Conn

    // State is the current state of the service.
    State string

    // LastRun is the last time a run of the service completed.
    LastRun time.Time

    // Active is a count of the number of active actions the service is processing.
    Active int
  }

Design your Actions

Actions signal the type of change you want to make to a field in the Container.
It consists of a two fields:

  Type: The type of change that is occurring, an int enum.
  Update: An optional field that can store any data used in the change.

Below is a set of functions that return an Action object.

  // The following are enumerators that refer to Action.Type that indicate what type
  // of action we are doing.
  // Note: Run the go stringer tool on the library for better output of these constants.
  const (
    // ActUnknown indicates this is not an Action we understand.
    ActUnknown = iota
    // ActAddConnection adds a GRPC connection.
    ActAddConnection
    // ActUpdateState updates the state of our application.
    ActUpdateState
    // ActUpdateRunTime updates Lastrun to indicate when the last time the application ran.
    ActUpdateRunTime
    // ActIncrementActive increments our Active counter.
    ActIncrementActive
    // ActDecrementActive decrements our Active counter.
    ActDecrementActive
  )

  // AddConnection adds a GRPC connection to a service to our state object.
  func AddConnection(name string, conn *grpc.Conn) Action {
    return Action{
      Type: ActAddConnection,
      Update: map[string]*grpc.Conn{name: conn}
    }
  }

  // UpdateState updates the state of the service in our state object.
  func UpdateState(state string) Action {
    return Action{
      Type: ActUpdateState,
      Update: state,
    }
  }

  // UpdateRunTime updates the last time our service completed an action to our state object.
  func UpdateRunTime(t time.Time) Action {
    return Action{
      Type: ActUpdateRunTime,
      Update: t,
    }
  }

  // IncrementActive updates the Active field to show that a new action is running.
  func IncrementActive() Action {
    return Action{
      Type: ActIncrementActive,
    }
  }

  // DecrementActive updates the Active field to show that an action has stopped running.
  func DecrementActive() Action {
    return Action{
      Type: ActDecrementActive,
    }
  }

Write your Updaters

Updaters are run against a copy of the current state and an Action and should
return the newly modified state. If the state is not modified by the Updater,
it must return the state object it received.

  // State updates our state object for ActUpdateState Actions.
  func State (state interface{}, action Action) interface{} {
    s := state.(MyState)

    switch action.Type {
    case ActUpdateState:
      s.State = action.Update.(string)
    }
    return s
  }

  // RunTime updates our state object for ActUpdateRunTime Actions.
  func RunTime (state interface{}, action Action) interface{} {
    s := state.(MyState)

    switch action.Type {
    case ActUpdateRunTime:
      s.RunTime = action.Update.RunTime.(time.Time)
    }
    return s
  }

  // Active updates our state object for ActIncrementActive and ActDecrementActive Actions.
  func Active (state interface{}, action Action) interface{} {
    s := state.(MyState)

    switch action.Type {
    case ActIncrementActive:
      s.Active++
    case ActDecrementActive:
      s.Active--
    }
    return s
  }

  // Connection updates our state object for ActAddConnection Actions.
  func Connection (state interface{}, action Action) interface{} {
    s := state.(MyState)

    switch action.Type {
    case ActAddConnection:
      // Because a map is a reference type, we must copy its values to a new map
      // and modify the new map.
      // Make a new map that has enough room for the change.
      n := make(map[string]*grpc.Conn, len(s.Connections)+1)

      // Copy the existing map into the new map.
      for k, v := range s.Connections {
        n[k] = v
      }

      // Copy the new value into the new map. Assume no duplicate keys.
      for k, v := range action.Update.(map[string]*grpc.Conn) {
        n[k] = v
      }

      // Do the assignment to the new state.
      s.Connections = n
    }
    return s
  }

Consolidate Updaters to a Modifier

A Modifier handles consolidating your Updaters for the Container.

  // mod will be used to run all of our Update objects on any change that is made.
  mod := NewModifier(Connection, State, RunTime, Active)

Create our Container object:

  // iniital is the initial state of the state object.
  var initial =  MyState {
    Connections: map[string]*grpc.Conn{},
    State: "not_started",
  }

  store, err := New(initial, mod)
  if err != nil {
    // Do something
  }

Subscribe to changes

Below we are subscriging to the "State" field when it changes and print out the change.
There is no guarenetee that you will receive all update messages, but when pulling from
the subscription channel, we guarenetee that it will be for the latest change.

  sub, cancel, err := store.Subscribe("State")
  if err != nil {
    // Do something
  }
  defer cancel()

  go func() {
    for s := range sub {
      fmt.Println(store.State().(MyState).State)
    }
  }()

Modify the state

The next code increments our counter, and waits for all changes.

  wg := &sync.WaitGroup{}
  go store.Perform(IncrementActive(), wg)
  go store.Perform(UpdateState("executing"), wg)
  go store.Perform(DecrementActive(), wg)

  wg.Wait()

This should output:

  executing

Retrieve the current state

It is safe to do read operations on this state object at any time without a lock.
However, you should never edit this object. Modifications should only happen via Container.Process().

  state  = store.Store().(MyState)
  fmt.Println(state.State)
  fmt.Println(state.Active)
*/
package boutique

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
)

// Any is used to indicated to Container.Subscribe() that you want updates for
// any update to the store, not just a field.
const Any = "any"

var (
	publicRE = regexp.MustCompile(`^[A-Z].*`)
)

// Signal is used to signal upstream subscribers that a field in the Container.Store
// has changed.
type Signal struct {
	// Version is the version of the field that was changed.  If Any was passed, it will
	// be the store's version, not a specific field.
	Version uint64

	// Fields are the field names that were updated.  This is only a single name unless
	// Any is used.
	Fields []string
}

// Action represents an action to take on the Container.
type Action struct {
	// Type should be an enumerated constant representing the type of Action.
	// It is valuable to use http://golang.org/x/tools/cmd/stringer to allow
	// for string representation.
	Type int

	// Update holds the values to alter in the Update.
	Update interface{}
}

// Updater takes in the existing state and an action to perform on the state.
// The result will be the new state.
// Implementation of an Updater must be careful to not mutate "state", it must
// work on a copy only. If you are changing a reference type contained in
// state, you must make a copy of that reference first and then manipulate
// the copy, storing it in the new state object.
type Updater func(state interface{}, action Action) interface{}

// Modifier provides the internals the ability to use the Updaters.
type Modifier struct {
	updater Updater
}

// NewModifier creates a new Modifier with the Updaters provided.
func NewModifier(updaters ...Updater) Modifier {
	return Modifier{updater: combineUpdater(updaters...)}
}

// run calls the updater on state/action.
func (m Modifier) run(state interface{}, action Action) interface{} {
	return m.updater(state, action)
}

// State holds the state data.
type State struct {
	// Version is the version of the state this represents.  Each change updates
	// this version number.
	Version uint64

	// FieldVersions holds the version each field is at. This allows us to track
	// individual field updates.
	FieldVersions map[string]uint64

	// Data is the state data.  They type is some type of struct.
	Data interface{}
}

// IsZero indicates that the State isn't set.
func (s State) IsZero() bool {
	if s.Data == nil {
		return true
	}
	return false
}

// GetState returns the state of the Store.
type GetState func() State

// Middleware provides a function that is called before the state is written.  The Action that
// is being applied is passed, with the newData that is going to be commited, a method to get the current state,
// and committed which will close when newData is committed. It returns either a changed version of newData or
// nil if newData is unchanged.  It returns an indicator if we should stop processing middleware but continue
// with our commit of the newData.  And it returns an error if we should not commit.
// Finally the "wg" WaitGroup that is passed must have .Done() called when the Middleware finishes.
// "committed" can be ignored unless the middleware wants to spin off a goroutine that does something after
// the data is committed.  If the data is not committed because another Middleware returns an error, the channel will
// be closed with an empty state. This ability allow Middleware that performs things such as logging the final result.
// If using this ability, do not call wg.Done() until all processing is done.
type Middleware func(a Action, newData interface{}, getState GetState, committed chan State, wg *sync.WaitGroup) (changedData interface{}, stop bool, err error)

// combineUpdater takes multiple Updaters and combines them into a
// single instance.
// Note: We do not provide any safety here. If you
func combineUpdater(updaters ...Updater) Updater {
	return func(state interface{}, action Action) interface{} {
		if err := validateState(state); err != nil {
			panic(err)
		}

		for _, u := range updaters {
			state = u(state, action)
		}
		return state
	}
}

// validateState validates that state is actually a Struct.
func validateState(state interface{}) error {
	if reflect.TypeOf(state).Kind() != reflect.Struct {
		return fmt.Errorf("a state may only be of type struct, which does not include *struct, was: %s", reflect.TypeOf(state).Kind())
	}
	return nil
}

// subscribers holds a mapping of field names to channels that will receive
// an update when the field name changes. A special field "any" will be updated
// for any change.
type subscribers map[string][]subscriber

type subscriber struct {
	id int
	ch chan Signal
}

type stateChange struct {
	old, new         interface{}
	newVersion       uint64
	newFieldVersions map[string]uint64
	changed          []string
}

// CancelFunc is used to cancel a subscription
type CancelFunc func()

func cancelFunc(c *Container, field string, id int) CancelFunc {
	return func() {
		c.smu.Lock()
		defer c.smu.Unlock()

		v := c.subscribers[field]
		if len(v) == 1 {
			close(v[0].ch)
			delete(c.subscribers, field)
			return
		}

		l := make([]subscriber, 0, len(v)-1)
		for _, s := range v {
			if s.id == id {
				close(s.ch)
				continue
			}
			l = append(l, s)
		}
		c.subscribers[field] = l
	}
}

// Container provides access to the single data store for the application.
// The Container is thread-safe.
type Container struct {
	// mod holds all the state modifiers.
	mod Modifier

	// middle holds all the Middleware we must apply.
	middle []Middleware

	// pmu prevents concurrent Perform() calls.
	pmu sync.Mutex

	// state is current state of the Container. Its value is a interface{}, so we
	// don't know the type, but it is guarenteed to be a struct.
	state atomic.Value

	// smu protects subscribers and sid.
	smu sync.RWMutex

	// subscribers holds the map of subscribers for different fields.
	subscribers subscribers

	// sid is an id for a subscriber.
	sid int
}

// New is the constructor for Container. initialState should be a struct that is
// used for application's state. All Updaters in mod must return the same struct
// that initialState contains or you will receive a panic.
func New(initialState interface{}, mod Modifier, middle []Middleware) (*Container, error) {
	if err := validateState(initialState); err != nil {
		return nil, err
	}

	if mod.updater == nil {
		return nil, fmt.Errorf("Modfifier must contain some Updaters")
	}

	fieldVersions := map[string]uint64{}
	for _, f := range fieldList(initialState) {
		fieldVersions[f] = 0
	}

	s := &Container{mod: mod, subscribers: subscribers{}, middle: middle}
	s.state.Store(State{Version: 0, FieldVersions: fieldVersions, Data: initialState})

	return s, nil
}

// Perform performs an Action on the Container's state. wg will be decremented
// by 1 to signal the completion of the state change. wg can be nil.
func (s *Container) Perform(a Action, wg *sync.WaitGroup) error {
	defer func() {
		if wg != nil {
			wg.Done()
		}
	}()

	s.pmu.Lock()
	defer s.pmu.Unlock()

	state := s.state.Load().(State)
	n := s.mod.run(state.Data, a)

	var (
		commitChans []chan State
		err         error
	)

	middleWg := &sync.WaitGroup{}
	middleWg.Add(len(s.middle))
	n, commitChans, err = s.processMiddleware(a, n, middleWg)
	if err != nil {
		for _, ch := range commitChans {
			close(ch)
		}
		return err
	}

	s.perform(state, n, commitChans)

	done := make(chan struct{})
	timer := time.NewTimer(5 * time.Second)
	go func() {
		middleWg.Wait()
		close(done)
	}()

	// This helps users diagnose misbehaving middleware.
	for {
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			glog.Infof("middleware is taking longer that 5 seconds, did you call wg.Done()?")
			continue
		}
		break
	}

	return nil
}

func (s *Container) processMiddleware(a Action, newData interface{}, wg *sync.WaitGroup) (data interface{}, commitChans []chan State, err error) {
	commitChans = make([]chan State, len(s.middle))
	for i := 0; i < len(commitChans); i++ {
		commitChans[i] = make(chan State, 1)
	}

	for i, m := range s.middle {
		cd, stop, err := m(a, newData, s.State, commitChans[i], wg)
		if err != nil {
			return nil, nil, err
		}

		if cd != nil {
			newData = cd
		}

		if stop {
			break
		}
	}
	return newData, commitChans, nil
}

func (s *Container) perform(state State, n interface{}, commitChans []chan State) {
	changed := fieldsChanged(state.Data, n)

	// This can happen if middleware interferes.
	if len(changed) == 0 {
		return
	}

	// Copy the field versions so that its safe between loaded states.
	fieldVersions := make(map[string]uint64, len(state.FieldVersions))
	for k, v := range state.FieldVersions {
		fieldVersions[k] = v
	}

	// Update the field versions that had changed.
	for _, k := range changed {
		fieldVersions[k] = fieldVersions[k] + 1
	}
	sort.Strings(changed)

	sc := stateChange{
		old:              state.Data,
		new:              n,
		newVersion:       state.Version + 1,
		newFieldVersions: fieldVersions,
		changed:          changed,
	}

	writtenState := s.write(sc)

	for _, ch := range commitChans {
		ch <- writtenState
	}
}

// write processes the change in state.
func (s *Container) write(sc stateChange) State {
	state := State{Data: sc.new, Version: sc.newVersion, FieldVersions: sc.newFieldVersions}
	s.state.Store(state)

	s.smu.RLock()
	defer s.smu.RUnlock()
	if len(s.subscribers) > 0 {
		go s.cast(sc)
	}
	return state
}

// Subscribe creates a subscriber to be notified when a field is updated.
// The notification comes over the returned channel.  If the field is set to
// the Any enumerator, any field change in the state data sends an update.
// CancelFunc() can be called to cancel the subscription. On cancel, Signal
// will be closed.
func (s *Container) Subscribe(field string) (chan Signal, CancelFunc, error) {
	if field != Any && !publicRE.MatchString(field) {
		return nil, nil, fmt.Errorf("cannot subscribe to a field that is not public: %s", field)
	}

	if field != Any && !fieldExist(field, s.State().Data) {
		return nil, nil, fmt.Errorf("cannot subscribe to non-existing field: %s", field)
	}

	ch := make(chan Signal, 1)

	s.smu.Lock()
	defer s.smu.Unlock()
	defer func() { s.sid++ }()

	if v, ok := s.subscribers[field]; ok {
		s.subscribers[field] = append(v, subscriber{id: s.sid, ch: ch})
	} else {
		s.subscribers[field] = []subscriber{
			{id: s.sid, ch: ch},
		}
	}
	return ch, cancelFunc(s, field, s.sid), nil
}

// State returns the current stored state.
func (s *Container) State() State {
	return s.state.Load().(State)
}

// cast updates subscribers for data changes.
func (s *Container) cast(sc stateChange) {
	s.smu.RLock()
	defer s.smu.RUnlock()

	for _, field := range sc.changed {
		if v, ok := s.subscribers[field]; ok {
			for _, sub := range v {
				signal(Signal{Version: sc.newFieldVersions[field], Fields: []string{field}}, sub.ch)
			}
		}
	}

	for _, sub := range s.subscribers["any"] {
		signal(Signal{Version: sc.newVersion, Fields: sc.changed}, sub.ch)
	}
}

// signal sends a Signa on a channel. If the channel is blocked, the signal is not sent.
func signal(sig Signal, ch chan Signal) {
	select {
	case ch <- sig:
		// Do nothing
	default:
		// Do nothing
	}
}

// fieldExists returns true if the field exists in "i".  This will panic if
// "i" is not a struct.
func fieldExist(f string, i interface{}) bool {
	return reflect.ValueOf(i).FieldByName(f).IsValid()
}

// fieldsChanged detects if a field changed between a and z. It reports that
// field name in the return. It is assumed a and z are the same type, if not
// this will not work correctly.
func fieldsChanged(a, z interface{}) []string {
	r := []string{}

	av := reflect.ValueOf(a)
	zv := reflect.ValueOf(z)

	for i := 0; i < av.NumField(); i++ {
		if av.Field(i).CanInterface() {
			if !reflect.DeepEqual(av.Field(i).Interface(), zv.Field(i).Interface()) {
				r = append(r, av.Type().Field(i).Name)
			}
		}
	}
	return r
}

// FieldList takes in a struct and returns a list of all its field names.
// This will panic if "st" is not a struct.
func fieldList(st interface{}) []string {
	v := reflect.TypeOf(st)
	sl := make([]string, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		sl[i] = v.Field(i).Name
	}
	return sl
}

// ShallowCopy makes a copy of a value. On pointers or references, you will
// get a copy of the pointer, not of the underlying value.
func ShallowCopy(i interface{}) interface{} {
	return i
}

// CopyAppendSlice takes a slice, copies the slice into a new slice and appends
// item to the new slice.  If slice is not actually a slice or item is not the
// same type as []Type, then this will panic.
// This is simply a convenience function for copying then appending to a slice.
// It is faster to do this by hand without the reflection.
func CopyAppendSlice(slice interface{}, item interface{}) interface{} {
	i, err := copyAppendSlice(slice, item)
	if err != nil {
		panic(err)
	}
	return i
}

// copyAppendSlice implements CopyAppendSlice, but with an error if there is
// a type mismatch. This makes it easier to test.
func copyAppendSlice(slice interface{}, item interface{}) (interface{}, error) {
	t := reflect.TypeOf(slice)
	if t.Kind() != reflect.Slice {
		return nil, fmt.Errorf("CopyAppendSlice 'slice' argument was a %s", reflect.TypeOf(slice).Kind())
	}
	if t.Elem().Kind() != reflect.TypeOf(item).Kind() {
		return nil, fmt.Errorf("CopyAppendSlice item is of type %s, but slice is of type %s", t.Elem(), reflect.TypeOf(item).Kind())
	}

	slicev := reflect.ValueOf(slice)
	var newcap, newlen int
	if slicev.Len() == slicev.Cap() {
		newcap = slicev.Len() + 1
		newlen = newcap
	} else {
		newlen = slicev.Len() + 1
		newcap = slicev.Cap()
	}

	ns := reflect.MakeSlice(slicev.Type(), newlen, newcap)

	reflect.Copy(ns, slicev)

	ns.Index(newlen - 1).Set(reflect.ValueOf(item))
	return ns.Interface(), nil
}
