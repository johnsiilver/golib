/*
Package boutique is a single state store for immutable objects inspired by flux
and redux. This allows for fast transforms and notifications of changes from a
single object.

Design your state object, which must be a struct (not a *struct):

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

Design your actions (run the stringer tool on the library):

  const (
    ActUnknown = iota
    ActAddConnection
    ActUpdateState
    ActUpdateRunTime
    ActIncrementActive
    ActDecrementActive
  )

  // AddConnection adds a GRPC connection to a service to our state object.
  func AddConnection(name string, conn *grpc.Conn) Action {
    return Action{
      Type: ActAddConnection,
      Update: MyState{
        Connections: map[string]*grpc.Conn{"name": conn},
      },
    }
  }

  // UpdateState updates the state of the service in our state object.
  func UpdateState(state string) Action {
    return Action{
      Type: ActUpdateState,
      Update: MyState{
        State: state,
      },
    }
  }

  // UpdateRunTime updates the last time our service completed an action to our state object.
  func UpdateRunTime(t time.Time) Action {
    return Action{
      Type: ActUpdateRunTime,
      Update: MyState{
        LastRun: t,
      },
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

Write your updaters:

  // Connection updates our state object for ActAddConnection Actions.
  func Connection (state interface{}, action Action) interface{} {
    // Always check out to state to make sure it exists.  If it doesn't, then
    // we need to provide an initial state.
    if state == nil {
      state = initial
    }

    switch action.Type {
    case ActAddConnection:
      // Make a copy of the state struct.
      s := ShallowCopy(state).(MyState)

      // Make a new map that has enough room for the change.
      n := make(map[string]*grpc.Conn, len(s.Connections)+1)

      // Copy the existing map data and the new data into the new map.
      for _, m := range []map[string]*grpc.Conn{action.Update.(MyState).Connections, state.(MyState).Connections} {
        for k, v := range m {
          n[k] = v
        }
      }

      // Do the assignment to the new state.
      s.Connections = n

      return s
    default:
      // If an Updater does not have an action, you must always return the same state you received.
      return state
    }
  }

  // State updates our state object for ActUpdateState Actions.
  func State (state interface{}, action Action) interface{} {
    switch action.Type {
    case ActUpdateState:
      s := ShallowCopy(state).(MyState)
      s.State = action.Update.(MyState).State
      return s
    default:
      // If an Updater does not have an action, you must always return the same state you received.
      return state
    }
  }

  // RunTime updates our state object for ActUpdateRunTime Actions.
  func RunTime (state interface{}, action Action) interface{} {
    switch action.Type {
    case ActUpdateRunTime:
      s := ShallowCopy(state).(MyState)
      s.RunTime = action.Update.(MyState).RunTime
      return s
    default:
      // If an Updater does not have an action, you must always return the same state you received.
      return state
    }
  }

  // Active updates our state object for ActIncrementActive and ActDecrementActive Actions.
  func Active (state interface{}, action Action) interface{} {
    switch action.Type {
    case ActIncrementActive:
      s := ShallowCopy(state).(MyState)
      s.Active = action.Update.(MyState).Active + 1
      return s
    case ActDecrementActive:
      s := ShallowCopy(state).(MyState)
      s.Active = action.Update.(MyState).Active - 1
      return s
    default:
      // If an Updater does not have an action, you must always return the same state you received.
      return state
    }
  }

Consolidate Updaters to a Modifier:

  // mod will be used to run all of our Update objects on any change that is made.
  mod := NewModifier(Connection, State, RunTime, Active)

Create our Store object:

  // iniital is the initial state of the state object.
  var initial =  MyState {
    Connections: map[string]*grpc.Conn{},
    State: "not_started",
  }

  store, err := New(initial, mod)
  if err != nil {
    // Do something
  }

Subscribe to the "State" field when it changes and print out the change:

  sub, err := store.Subscribe("State")
  if err != nil {
    // Do something
  }

  go func() {
    for s := range sub {
      fmt.Println(store.State().(MyState).State)
    }
  }()

Modify the State field:

  store.Perform(IncrementActive())
  store.Perform(UpdateState("excuting"))
  store.Perform(DecrementActive())
*/
package boutique

import (
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"sync/atomic"
)

// Any is used to indicated to Store.Subscribe() that you want updates for
// any update to the store, not just a field.
const Any = "any"

var (
	public = regexp.MustCompile(`^[A-Z].*`)
)

// Action represents an action to take on the Store.
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
// If that object requires a deepcopy, use https://github.com/mohae/deepcopy .
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
type subscribers map[string][]chan struct{}

type stateChange struct {
	old, new interface{}
	wg       *sync.WaitGroup
}

// Option is an option for the constructor New().
type Option func(s *Store)

// WriteBuffer allows a user to change the size of the write buffer in the Store.
// The write buffer is sized at 100, allowing 100 concurrent Perform() calls
// that are non-blocking.
func WriteBuffer(size int) Option {
	return func(s *Store) {
		s.scChBuffer = size
	}
}

// Store provides access to the single data store for the application.
// The Store is thread-safe.
type Store struct {
	// mod is holds all the state modifiers.
	mod Modifier

	// smu prevents concurrent AddSubscriber() calls.
	smu sync.Mutex
	// subscribers holds the map of subscribers for different fields.
	// It stores type subscribers.
	subscribers atomic.Value

	// writeCh is where changes to the state are sent and processed.
	writeCh chan stateChange

	// scChBuffer is the amount of buffer to give writeCh.
	scChBuffer int
	// state is current state of the Store. Its value is a interface{}, so we
	// don't know the type, but it is guarenteed to be a struct.
	state atomic.Value
}

// New is the constructor for Store.
func New(initialState interface{}, mod Modifier, opts ...Option) (*Store, error) {
	if err := validateState(initialState); err != nil {
		return nil, err
	}

	if mod.updater == nil {
		return nil, fmt.Errorf("Modfifier must contain some Updaters")
	}

	s := &Store{mod: mod, scChBuffer: 100}
	for _, opt := range opts {
		opt(s)
	}

	s.writeCh = make(chan stateChange, s.scChBuffer)
	return s, nil
}

// Perform performs an Action on the Store's state. wg will be decremented
// by 1 to signal the completion of the state change. wg can be nil.
func (s *Store) Perform(a Action, wg *sync.WaitGroup) {
	state := s.state.Load()
	n := s.mod.run(state, a)

	s.writeCh <- stateChange{old: state, new: n, wg: wg}
}

// write loops on the writeCh and processes the change.
func (s *Store) write() {
	for sc := range s.writeCh {
		s.state.Store(sc.new)
		if sc.wg != nil {
			sc.wg.Done()
		}

		go s.cast(sc.old, sc.new)
	}
}

// Subscribe creates a subscriber to be notified when a field is updated.
// The notification comes over the returned channel.  If the field is set to
// the Any enumerator, any field change in the state data sends an update.
func (s *Store) Subscribe(field string) (chan struct{}, error) {
	if !public.MatchString(field) {
		return nil, fmt.Errorf("cannot subscribe to a field that is not public: %s", field)
	}

	if !fieldExist(field, s.State()) {
		return nil, fmt.Errorf("cannot subscribe to non-existing field: %s", field)
	}

	ch := make(chan struct{}, 1)

	s.smu.Lock()
	defer s.smu.Unlock()

	subs := s.subscribers.Load().(subscribers)
	if v, ok := subs[field]; ok {
		subs[field] = append(v, ch)
	} else {
		subs[field] = []chan struct{}{ch}
	}
	return ch, nil
}

// State returns the current stored state.
func (s *Store) State() interface{} {
	return s.state.Load()
}

// cast updates subscribers for data changes.
func (s *Store) cast(a, z interface{}) {
	changed := fieldsChanged(a, z)
	sub := s.subscribers.Load().(subscribers)
	for _, field := range changed {
		if v, ok := sub[field]; ok {
			for _, ch := range v {
				signal(ch)
			}
		}
	}
	for _, ch := range sub["any"] {
		signal(ch)
	}
}

// signal sends an empty struct as a signal on a channel. If the channel is
// blocked, the signal is not sent.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
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

// ShallowCopy makes a copy of a value. On pointers or references, you will
// get a copy of the pointer, not of the underlying value.
func ShallowCopy(i interface{}) interface{} {
	return i
}
