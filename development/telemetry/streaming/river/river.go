// Package river provides a streaming version of the Go standard library
// expvar.  This allows monitors to be updated to only variables they are
// interested in and only when the variable has actually changed.
package river

import (
	"errors"
	"expvar"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"
)

var (
	// reMu protects everything below.
	regMu sync.Mutex

	// globalID is what is used to inform the monitor who is communicating with them.
	globalID ID

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

// IDAttr is an attribute used to make up an ID.
type IDAttr struct {
	// Field is the name of the field.
	Field string
	// Value is the string value of the field.
	Value string
}

func (i IDAttr) validate() error {
	if i.Field == "" {
		return errors.New("cannot have IDAttr with Field set to empty string")
	}
	if strings.Contains(i.Value, ":") {
		return fmt.Errorf("IDAttr with field %s, cannot have value with ':' in it", i.Field)
	}
	return nil
}

// ID provides an ID for the application instance, which must be unique between
// apps talking to a monitor.
type ID interface {
	// Attributes returns a list of field/value pairs that are used to make up
	// an ID.  A string version of this IDAttr will be output using this order
	// with just the values separated by ":".  ":"'s a'
	Attributes() []IDAttr
}

func IDString(id ID) (string, error) {
	attrs := id.Attributes()
	if len(attrs) == 0 {
		return "", fmt.Errorf("no attributes in the ID")
	}

	var out []string
	m := map[string]bool{}

	for _, attr := range id.Attributes() {
		if err := attr.validate(); err != nil {
			return "", err
		}
		if m[attr.Field] {
			return "", fmt.Errorf("cannot turn an ID to a string: field %s was repeated in Attributes()", attr.Field)
		}
		out = append(out, attr.Value)
	}
	return strings.Join(out, ":"), nil
}

// SetID sets the service's identifier to the monitors.  This must be unique
// to your monitors or they will reject the conneciton attempts.
// Must be set before calling Publish() or RegisterMonitor().
// Calling this twice will panic.
func SetID(id ID) {
	regMu.Lock()
	defer regMu.Unlock()

	if _, err := IDString(id); err != nil {
		panic(err)
	}

	globalID = id
}

// Publish declares a named exported variable. This should be called from a
// package's init function when it creates its Vars. If the name is already
// registered then this will log.Panic.
// Publishing a Func will call the Func() once to verify its output.
func Publish(name string, v expvar.Var) {
	regMu.Lock()
	defer regMu.Unlock()

	if globalID == nil {
		panic("must call SetID() before Publish()")
	}

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
