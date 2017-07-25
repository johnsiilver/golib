// Package river provides a streaming version of the Go standard library
// expvar.  This allows monitors to be updated to only variables they are
// interested in and only when the variable has actually changed.
package river

import (
	"expvar"
	"log"
	"sort"
	"sync"
	"sync/atomic"
)

var (
	// reMu protects registry
	regMu sync.Mutex

	// registry holds our mapping of expvar Variables.
	registry atomic.Value // map[string]expvar.Var
)

func getRegistry() map[string]expvar.Var {
	return registry.Load().(map[string]expvar.Var)
}

// Publish declares a named exported variable. This should be called from a
// package's init function when it creates its Vars. If the name is already
// registered then this will log.Panic.
// Publishing a Func will call the Func() once to verify its output.
func Publish(name string, v expvar.Var) {
	regMu.Lock()
	defer regMu.Unlock()

	r := getRegistry()
	if _, ok := r[name]; ok {
		log.Panicf("river.Publish(): two packages tried to register variable %s", name)
	}

	switch /*t :=*/ v.(type) {
	case Float, Int, String, *Map:
		/*
			case Func:
				ret := t.Value()
				switch ret.(type) {
				case int64, float64, string, Map, expvar.Var:
				default:
					panic("cannot Publish a Func() that does not return int64/float64/string/river.Map/expvar.Var")
				}
				v = newFuncHolder(t)
		*/
	default:
		panic("cannot Publish an expvar.Var not defined in the river pacakge")
	}

	r[name] = v
	registry.Store(r)
}

// GetVars returns all the registered variable names.
func GetVars() []string {
	r := getRegistry()
	sl := make([]string, len(r))
	for k := range r {
		sl = append(sl, k)
	}
	sort.Strings(sl)
	return sl
}
