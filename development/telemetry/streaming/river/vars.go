package river

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/actions"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
)

const (
	unknownVar = "unknown"
)

// Var is an abstract type for all exported variables.
type Var interface {
	// String returns a valid JSON value for the variable.
	// Types with String methods that do not return valid JSON
	// (such as time.Time) must not be used as a Var.
	String() string

	// varState returns the current data.VarState that is held internally.
	varState() data.VarState

	// subscribe allows internal objects to subscribe to changes to the variable.
	subscribe() (chan boutique.Signal, boutique.CancelFunc)

	// isRiverVar lets us know its a river.Var instead of expvar.Var.
	isRiverVar()

	// setName allows us to change the internal name of the variable.
	setName(n string)
}

// Int is a 64-bit integer variable that satisfies the Var interface.
type Int struct {
	store *boutique.Store
}

// MakeInt is creates a new Int, but does not call Publish() like NewInt().
func MakeInt(i int64) Int {
	v := Int{store: state.NewVarState("unnamed")}
	v.Set(i)
	return v
}

// NewInt creates a new Int and calls Publish(name, Int).
func NewInt(name string) Int {
	i := MakeInt(0)
	Publish(name, i)
	return i
}

func (v Int) Value() int64 {
	return v.store.State().Data.(data.VarState).Int
}

func (v Int) String() string {
	return strconv.FormatInt(v.Value(), 10)
}

func (v Int) Add(delta int64) {
	v.store.Perform(actions.IntAdd(delta))
}

func (v Int) Set(value int64) {
	v.store.Perform(actions.IntSet(value))
}

func (v Int) subscribe() (chan boutique.Signal, boutique.CancelFunc) {
	sig, cancel, err := v.store.Subscribe("Int")
	if err != nil {
		panic(err) // This should never happen.
	}
	return sig, cancel
}

func (v Int) varState() data.VarState {
	return getData(v.store)
}

func (v Int) isRiverVar() {}

func (v Int) setName(n string) {
	v.store.Perform(actions.NameSet(n))
}

// Float is a 64-bit float variable that satisfies the Var interface.
type Float struct {
	store *boutique.Store
}

// MakeFloat creates a new Float, but does not call Publish() like NewFloat().
func MakeFloat(f float64) Float {
	v := Float{store: state.NewVarState(unknownVar)}
	v.Set(f)
	return v
}

// NewFloat makes a new Float and calls Publish(name, Float) with it.
func NewFloat(name string) Float {
	f := MakeFloat(0)
	Publish(name, f)
	return f
}

func (v Float) Value() float64 {
	return v.store.State().Data.(data.VarState).Float
}

func (v Float) String() string {
	return strconv.FormatFloat(v.Value(), 'g', -1, 64)
}

// Add adds delta to v.
func (v Float) Add(delta float64) {
	v.store.Perform(actions.FloatAdd(delta))
}

// Set sets v to value.
func (v Float) Set(value float64) {
	v.store.Perform(actions.FloatSet(value))
}

func (v Float) subscribe() (chan boutique.Signal, boutique.CancelFunc) {
	sub, cancel, err := v.store.Subscribe("Float")
	if err != nil {
		panic(err)
	}
	return sub, cancel
}

func (v Float) varState() data.VarState {
	return getData(v.store)
}

func (v Float) isRiverVar() {}

func (v Float) setName(n string) {
	v.store.Perform(actions.NameSet(n))
}

// String is a string variable, and satisfies the Var interface.
type String struct {
	store *boutique.Store
}

func MakeString(s string) String {
	v := String{store: state.NewVarState(unknownVar)}
	v.Set(s)
	return v
}

// NewString is the constructor for String.
func NewString(name string) String {
	s := MakeString("")
	Publish(name, s)
	return s
}

func (v String) Value() string {
	return v.store.State().Data.(data.VarState).String
}

// String implements the Val interface. To get the unquoted string
// use Value.
func (v String) String() string {
	b, _ := json.Marshal(v.Value())
	return string(b)
}

func (v String) Set(value string) {
	v.store.Perform(actions.String(value))
}

func (v String) subscribe() (chan boutique.Signal, boutique.CancelFunc) {
	sub, cancel, err := v.store.Subscribe("String")
	if err != nil {
		panic(err)
	}
	return sub, cancel
}

func (v String) varState() data.VarState {
	return getData(v.store)
}

func (v String) isRiverVar() {}

func (v String) setName(n string) {
	v.store.Perform(actions.NameSet(n))
}

type subscription struct {
	sub    chan boutique.Signal
	cancel boutique.CancelFunc
}

// Map is a string-to-Var map variable that satisfies the Var interface.
type Map struct {
	// store is unique here.  There is no guarentee of immutability because of
	// the nature of this Map.  There is no good way to do that without changing
	// the API, which is not desirable.  This is fine in this case, where our
	// major concern is notifiying our monitors.
	store *boutique.Store

	// addSub is used to subscribe to changes in map values.
	addSub chan subscription
	// subscriptions contains all the value subscriptions.
	// This is not used, but it seemed wise to keep track of CancelFuncs.
	subscriptions []subscription

	mu sync.Mutex
}

// MakeMap creates a new *Map, but does not call Publish() like NewMap().
func MakeMap() *Map {
	v := &Map{
		store:         state.NewVarState(unknownVar),
		addSub:        make(chan subscription, 10),
		subscriptions: []subscription{},
		mu:            sync.Mutex{},
	}

	go v.subs()

	return v
}

func (v *Map) subs() {
	const newSub = 0

	cases := []reflect.SelectCase{
		// #0: <-v.addSub
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(v.addSub),
		},
	}

	for {
		chosen, val, _ := reflect.Select(cases)

		switch chosen {
		case newSub:
			sub := val.Interface().(subscription)
			v.subscriptions = append(v.subscriptions, sub)

			cases = append(
				cases,
				reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(sub.sub),
				},
			)
		default:
			v.store.Perform(actions.NoOp())
		}
	}
}

// NewMap is the constructor for Map.
func NewMap(name string) *Map {
	m := MakeMap()
	Publish(name, m)
	return m
}

func (v *Map) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{")
	first := true
	v.Do(func(kv expvar.KeyValue) {
		if !first {
			fmt.Fprintf(&b, ", ")
		}
		fmt.Fprintf(&b, "%q: %v", kv.Key, kv.Value)
		first = false
	})
	fmt.Fprintf(&b, "}")
	return b.String()
}

func (v *Map) Get(key string) expvar.Var {
	m := v.store.State().Data.(data.VarState).Map
	return m[key]
}

func (v *Map) Set(key string, av Var) {
	val := av.(Var)
	sub := subscription{}
	sub.sub, sub.cancel = val.subscribe()
	v.addSub <- sub
	v.store.Perform(actions.StoreMap(key, av))
}

func (v *Map) Add(key string, delta int64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.store.State().Data.(data.VarState)
	val := state.Map[key]

	if val == nil {
		i := MakeInt(delta)
		sub := subscription{}
		sub.sub, sub.cancel = i.subscribe()
		v.addSub <- sub
		v.store.Perform(actions.StoreMap(key, i))
		return
	}

	// Ignore if its not an Int.
	if i, isInt := val.(Int); isInt {
		i.Add(delta)
	}
}

// AddFloat adds delta to the *Float value stored under the given map key.
func (v *Map) AddFloat(key string, delta float64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.store.State().Data.(data.VarState)
	val := state.Map[key]

	if val == nil {
		f := MakeFloat(delta)
		sub := subscription{}
		sub.sub, sub.cancel = f.subscribe()
		v.addSub <- sub
		v.store.Perform(actions.StoreMap(key, f))
		return
	}
	// Ignore if its not an Float.
	if f, isFloat := val.(Float); isFloat {
		f.Add(delta)
	}
}

// Do calls f for each entry in the map.
// The map is locked during the iteration,
// but existing entries may be concurrently updated.
func (v *Map) Do(f func(expvar.KeyValue)) {
	m := v.store.State().Data.(data.VarState).Map
	for k, v := range m {
		f(expvar.KeyValue{Key: k, Value: v})
	}
}

// Init is a no-op and is included here only for compatibility.
func (v *Map) Init() *Map {
	return v
}

func (v *Map) subscribe() (chan boutique.Signal, boutique.CancelFunc) {
	sub, cancel, err := v.store.Subscribe(boutique.Any)
	if err != nil {
		panic(err)
	}
	return sub, cancel
}

func (v *Map) varState() data.VarState {
	return getData(v.store)
}

func (v *Map) isRiverVar() {}

func (v *Map) setName(n string) {
	v.store.Perform(actions.NameSet(n))
}

/*
// Func implements Var by calling the function. Unlike the standard expvar.Func,
// this one is more picky with what you can return. You can return the following
// types: int64, float64, string, river.Map, or expvar.Var.  In the case of
// expvar.Var, that value will be returned as a string.
// You must always return the same type.  Not doing so will cause this to have
// unknown behavior.
type Func func() interface{}

func (f Func) Value() interface{} {
	return f()
}

func (f Func) String() string {
	v, _ := json.Marshal(f())
	return string(v)
}

// funcHolder implements expvar.Var. It provides methods to allow for using the
// Func() at intervals.
type funcHolder struct {
	f          Func
	store      *boutique.Store
	outType    data.VarType
	intervalCh chan time.Duration
	stopCh     chan struct{}

	expvar.Var // Satifisfies expvar.Var

	sync.Mutex // Protects variables below
	running    bool
}

func newFuncHolder(f Func) *funcHolder {
	store, err := boutique.New(data.VarState{}, modifiers.All, nil)
	if err != nil {
		panic("wtf")
	}

	fu := &funcHolder{
		f:          f,
		store:      store,
		intervalCh: make(chan time.Duration),
		stopCh:     make(chan struct{}),
	}

	d := fu.run()
	fu.outType = d.Type
	return fu
}

// run is used to execute the internal function and return output.
func (f *funcHolder) run() data.VarState {
	v := f.f()
	var d data.VarState

	switch t := v.(type) {
	case int64:
		d = data.VarState{
			Type: data.IntType,
			Int:  t,
		}
	case float64:
		d = data.VarState{
			Type:  data.FloatType,
			Float: t,
		}
	case string:
		d = data.VarState{
			Type:   data.StringType,
			String: t,
		}
	case Map:
		d = data.VarState{
			Type: data.MapType,
			Map:  t.store.State().Data.(data.VarState).Map,
		}
	case expvar.Var:
		d = data.VarState{
			Type:   data.StringType,
			String: t.String(),
		}
	default:
		if st, ok := v.(fmt.Stringer); ok {
			d = data.VarState{
				Type:   data.StringType,
				String: st.String(),
			}
		} else {
			b, err := json.Marshal(v)
			if err != nil {
				panic("a Func was used that doesn't return a type that is known, does not have a Stringer method, or cannot be JSON encoded")
			}
			d = data.VarState{
				Type:   data.StringType,
				String: string(b),
			}
		}
	}
	return d
}

// stop stops the data gathering.
func (f *funcHolder) stop() {
	f.Lock()
	defer f.Unlock()
	if !f.running {
		return
	}
	f.stopCh <- struct{}{}
}

// loop the function at a minimum of interval and updates a
// boutique store with the function output.
// If called while another execute() is running, it will not run again.  It will
// adjuct the interval if the interval is different.
// If the interval is negative, it will execute the loop only once.
func (f *funcHolder) loop(interval time.Duration) {
	// TODO(johnsiilver): Refactor, this is ugly.
	shouldReturn := func() bool {
		f.Lock()
		defer f.Unlock()
		if f.running {
			f.intervalCh <- interval // Maybe they are trying to change the interval
			return true
		}
		return false
	}()
	if shouldReturn {
		return
	}

	defer func() {
		f.Lock()
		defer f.Unlock()
		f.running = false
	}()

	var last time.Time

	for {
		// Sleep until the next interval.
		curInterval := time.Now().Sub(last)
		select {
		case <-f.stopCh:
			return
		case <-time.After(interval - curInterval): // Wait our interval.
			// Do nothing
		case d := <-f.intervalCh: // Reset our interval and start the wait.
			if d == interval { // They were not changing the interval.
				continue
			}
			interval = d
			continue
		}

		d := f.run()
		switch d.Type {
		case data.IntType:
			f.store.Perform(actions.IntSet(d.Int))
		case data.FloatType:
			f.store.Perform(actions.FloatSet(d.Float))
		case data.StringType:
			f.store.Perform(actions.String(d.String))
		case data.MapType:
			f.store.Perform(actions.ReplaceMap(d.Map))
		default:
			panic("a Func was used that doesn't return an int64/float64/string/Map/expvar.Var. Should have been caught on Publish()")
		}
		last = time.Now()
		if interval < 0 {
			return
		}
	}
}

// Get returns the current value.
func (v funcHolder) Value() interface{} {
	return getData(v.store).Value()
}

// Subscribe subscribes to updates.
func (v funcHolder) Subscribe() (chan boutique.Signal, boutique.CancelFunc, error) {
	sub, err := getData(v.store).ValueType().SubString()
	if err != nil {
		return nil, nil, err
	}
	return v.store.Subscribe(sub)
}
*/

func getData(store *boutique.Store) data.VarState {
	return store.State().Data.(data.VarState)
}
