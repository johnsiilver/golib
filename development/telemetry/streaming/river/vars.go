package river

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"strconv"
	"sync"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/actions"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
)

// Var is an abstract type for all exported variables.
type Var interface {
	// String returns a valid JSON value for the variable.
	// Types with String methods that do not return valid JSON
	// (such as time.Time) must not be used as a Var.
	String() string

	// IsRiverVar lets us know its a river.Var instead of expvar.Var.
	IsRiverVar()
}

// Int is a 64-bit integer variable that satisfies the Var interface.
type Int struct {
	store *boutique.Store
}

func newInt(i int64) Int {
	v := Int{store: state.NewVarState()}
	v.Set(i)
	return v
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

// Float is a 64-bit float variable that satisfies the Var interface.
type Float struct {
	store *boutique.Store
}

func newFloat(f float64) Float {
	v := Float{store: state.NewVarState()}
	v.Set(f)
	return v
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

// Map is a string-to-Var map variable that satisfies the Var interface.
type Map struct {
	store *boutique.Store
	mu    *sync.RWMutex
}

func newMap(m map[string]expvar.Var) Map {
	v := Map{store: state.NewVarState(), mu: &sync.RWMutex{}}
	if m != nil {
		v.store.Perform(actions.ReplaceMap(m))
	}
	return v
}

func (v Map) String() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
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

func (v Map) Get(key string) expvar.Var {
	v.mu.RLock()
	defer v.mu.RUnlock()
	m := v.store.State().Data.(data.VarState).Map
	return m[key]
}

func (v Map) Set(key string, av expvar.Var) {
	switch av.(type) {
	case Float, Int, Func, String:
	default:
		panic("river.Map.Set can only store expvar.Var defined in expvar")
	}
	v.store.Perform(actions.StoreMap(key, av))
}

func (v Map) Add(key string, delta int64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.store.State().Data.(data.VarState)
	val := state.Map[key]
	// Ignore if its not an *expvar.Int.
	if _, isInt := val.(*expvar.Int); isInt {
		return
	}

	if val == nil {
		v.store.Perform(actions.IntSet(delta))
		return
	}

	v.store.Perform(actions.IntAdd(delta))
}

// AddFloat adds delta to the *Float value stored under the given map key.
func (v Map) AddFloat(key string, delta float64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.store.State().Data.(data.VarState)
	val := state.Map[key]

	// Ignore if its not an *expvar.Int.
	if _, isFloat := val.(*expvar.Float); isFloat {
		return
	}

	if val == nil {
		v.store.Perform(actions.FloatSet(delta))
		return
	}

	v.store.Perform(actions.FloatAdd(delta))
}

// Do calls f for each entry in the map.
// The map is locked during the iteration,
// but existing entries may be concurrently updated.
func (v Map) Do(f func(expvar.KeyValue)) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	m := v.store.State().Data.(data.VarState).Map
	for k, v := range m {
		f(expvar.KeyValue{k, v})
	}
}

// String is a string variable, and satisfies the Var interface.
type String struct {
	store *boutique.Store
}

func newString(s string) String {
	v := String{store: state.NewVarState()}
	v.Set(s)
	return v
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

// Func implements Var by calling the function
// and formatting the returned value using JSON.
type Func func() interface{}

func (f Func) Value() interface{} {
	return f()
}

func (f Func) String() string {
	v, _ := json.Marshal(f())
	return string(v)
}

// All published variables.
var (
	mutex   sync.RWMutex
	vars    = make(map[string]Var)
	varKeys []string // sorted
)
