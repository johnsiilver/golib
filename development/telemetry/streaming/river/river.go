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

	"github.com/golang/glog"
)

var (
	// reMu protects registry
	regMu sync.Mutex

	// registry holds our mapping of expvar Variables.
	registry atomic.Value // map[string]expvar.Var
)

func getRegistry() map[string]expvar.Var {
	m := registry.Load()
	if m == nil {
		return nil
	}
	return m.(map[string]expvar.Var)
}

// Publish declares a named exported variable. This should be called from a
// package's init function when it creates its Vars. If the name is already
// registered then this will log.Panic.
// Publishing a Func will call the Func() once to verify its output.
func Publish(name string, v expvar.Var) {
	regMu.Lock()
	defer regMu.Unlock()

	r := getRegistry()
	if r == nil {
		r = map[string]expvar.Var{}
	}

	if _, ok := r[name]; ok {
		log.Panicf("river.Publish(): two packages tried to register variable %s", name)
	}

	if t, ok := v.(Var); !ok {
		log.Panicf("river.Publish(%s): all published variables must be from the river package, was %T", name, v)
	} else {
		t.setName(name)
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
	glog.Infof("registering variable %s of type %T", name, v)
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
