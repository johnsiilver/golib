/*
Package statemachine provides a generalized state machine. It is based on a talk by Rob Pike (though I'm sure any similar
implementation by him would be infinitely better).

This statemachine does not use a state type to go from state to state, but instead uses state functions to determine the next
state to execute directly.

Example usage:
  // myStateObj is a simple object holding our various StateFn's for our state machine. We could have done this
  // example with just functions instead of methods, but this is to show you can do the same with objects holding
  // attributes you want to maintain through execution.
  type myStateObj struct {}

  // PrintHello implements StateFn. This will be our starting state.
  func (s StateObj) PrintHello() (StateFn, error) {
    fmt.Println("Hello ")
    return s.PrintWorld, nil
  }

  // PrintWorld implements StateFn.
  func (s StateObj) PrintWorld() (StateFn, error) {
    fmt.Println("World")
    return nil, nil
  }

  func main() {
    so := myStateObj{}

    // Creates a new statemachine executor that will start execution with myStateObj.PrintHello().
    exec := statemachine.New("helloWorld", so.PrintHello)

    // This begins execution and gets our final error state.
    if err := exec.Execute(); err != nil {
      // Do something with the error.
    }
  }

You may also be interested in having the Executor give internal run state information back to you. This is maintained
inside the Executor and may be accessed by a call to Executor.Nodes() which will print out all the StateFn's called.

If you would like to have a running diagnostic mixed with your other logs, you can do the following:
  // Provide your favorite logger for us to use.
  log := func(s string, i ...interface{}) {
    glog.Infof(s, i...)  // glog is a logging package, see https://github.com/golang/glog
  }

  exec := statemachine.New("helloWorld", so.PrintHello, statemachine.LogFacility(log))
  exec.Log(true)
*/
package statemachine

import (
	"fmt"
  "reflect"
	"runtime"
  "strings"
	"sync"
)

// StateFn represents a function that executes at a given state.  "s" represents the new state designation.
type StateFn func() (StateFn, error)

// LogFn represents some logging function to handle logging when Executor.Logging(true) is set. It should do
// variable substituion similar to fmt.Sprintf() does.
type LogFn func(s string, i ...interface{})

// Executor provides methods for executing a state machine. These methods are not thread safe.
type Executor interface {
	// Execute executes the statemachine. It stops the first time a StateFn returns an error or returns "nil" for the returned
	// StateFn. If the last StateFn returned an error, StateFn returns an error. Execute() caused the internal state to be cleared
	// and calls the function provided to New() by the Reset option, if provided.
	Execute() error

	// Nodes returns a list of the StateFn's that were executed during the last call to Execute().
	Nodes() []string

	// Log turns on/off detailed logging of the execution state. To use this you must have provided New() with the LogFacility() option.
	Log(b bool)
}

// Option provides an optional argument for New().
type Option func(e *executor)

// Reset provides a function that is called when Executor.Reset() is called. This function should reset any data
// needed by StateFn's used in the Executor.
func Reset(f func()) Option {
	return func(e *executor) {
		e.resetFn = f
	}
}

// LogFacility sets up the internal log function for Executor for when Executor.Log(true) is called.
func LogFacility(l LogFn) Option {
	return func(e *executor) {
		e.logger = l
	}
}

// New is the constructor for Executor. "start" is the StateFn that is first called when Executor.Execute() is called.
// "name" is used to prepend logging messages as a unique identifier.
func New(name string, start StateFn, opts ...Option) Executor {
	e := &executor{name: name, startFn: start}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// executor implements Executor.
type executor struct {
	// name is the name of this particular executor.
	name string

	// startFn is the StateFn that starts the execution.
	startFn StateFn

	// currentFn is the StateFn that is currently being executed.
	currentFn StateFn

	// nodes are all the StateFn names executed during the last execution.
	nodes []string

	// resetFn is a function provided by the user that
	resetFn func()

	// logger holds a function for handling logging messages.
	logger LogFn

	// logOn indicates if we should be logging internal statemachine executions.
	logOn bool

	// used to protect nodes, currentFn, and logOn.
	sync.Mutex
}

// Execute implements Executor.Execute.
func (e *executor) Execute() error {
	defer func() {
		if e.logOn {
			e.log("The following is the StateFn's called with this execution:")
			for _, node := range e.Nodes() {
				e.log("\t%s", node)
			}
		}
	}()

	e.reset()

	f := e.startFn
	var err error

	for {
		f, err = e.fnWrapper(f)

		switch {
		case f == nil && err == nil:
			e.log("Execute() completed with no issues")
			return nil
		case err != nil:
			e.log("Execute() completed with an error: %q", err)
			return err
		}
	}
}

func (e *executor) reset() {
	e.Lock()
	defer e.Unlock()

	e.nodes = make([]string, 0, 20)
	e.currentFn = nil

	if e.resetFn != nil {
		e.resetFn()
	}
}

// fnWrapper does some internal tracking before execute "f".
func (e *executor) fnWrapper(f StateFn) (StateFn, error) {
	e.Lock()
  name := fNameScrub(f)
	e.nodes = append(e.nodes, name)
	e.currentFn = f
	e.Unlock()

	e.log("StateFn(%s) starting", name)

	fn, err := f()

	e.log("StateFn(%s) finished", name)

	return fn, err
}

// Nodes implements Executor.Nodes().
func (e *executor) Nodes() []string {
	e.Lock()
	defer e.Unlock()

	return e.nodes
}

// Log implements Executor.Log().
func (e *executor) Log(b bool) {
	if e.logger == nil {
		return
	}

	e.Lock()
	defer e.Unlock()
	e.logOn = b
}

func (e *executor) log(s string, i ...interface{}) {
	if e.logOn && e.logger != nil {
		e.logger(fmt.Sprintf("StateMachine[%s]: %s", e.name, s), i...)
	}
}
// fNameScrub gets the name of funtion "f", removes package information and trailing stuff we
// don't care about and returns it.
func fNameScrub(f StateFn) string {
  v := reflect.ValueOf(f)
  pc := runtime.FuncForPC(v.Pointer())
  return fScrub(pc.Name())
}

// fScrub does the actual name scrub for fNameScrub. It is split out to allow the tests to scrub
// the name. The tests use a different way to get the function name.
func fScrub(s string) string {
  sp := strings.SplitAfter(s, ".")
  return strings.TrimSuffix(sp[len(sp)-1], "-fm")
}

// MockExecutor implements Executor. It can be used in tests where you only need to test something happened.
// It will return "ReturnVal" and will call "SideEffect" before it completes. You should always use MockExecutor
// instead of your own fake to prevent interface changes during updates from breaking your code.
type MockExecutor struct {
	// ReturnVal is the response you wish to receive from Execute().
	ReturnVal error

	// SideEffect causes some function to run at the end of Execute. You can use this to make changes to other objects
	// that would have been affected by the real execution.
	SideEffect func()

	// NodesVal is a list of StateFn's that you wish to indicate were executed.
	NodesVal []string
}

// Execute implements Executor.Execute().
func (m *MockExecutor) Execute() error {
	if m.SideEffect != nil {
		defer m.SideEffect()
	}
	return m.ReturnVal
}

// Nodes implements Executor.Nodes().
func (m *MockExecutor) Nodes() []string {
	return m.NodesVal
}

// Logging implements Executor.Logging().
func (m *MockExecutor) Log(b bool) {}
