package statemachine

import (
	"fmt"
	"runtime"
  "strings"
	"testing"

	"github.com/kr/pretty"
)

// StateMachine provides a simple state machine for testing the Executor.
type StateMachine struct {
	err       bool
	callTrace []string
}

// Start implements StateFn.
func (s *StateMachine) Start() (StateFn, error) {
	s.trace()
	return s.Middle, nil
}

// Middle implements StateFn.
func (s *StateMachine) Middle() (StateFn, error) {
	s.trace()
	if s.err {
		return s.Error, nil
	}
	return s.End, nil
}

// End implements StateFn.
func (s *StateMachine) End() (StateFn, error) {
	s.trace()
	return nil, nil
}

// Error implements StateFn.
func (s *StateMachine) Error() (StateFn, error) {
	s.trace()
	return nil, fmt.Errorf("error")
}

func (s *StateMachine) reset() {
	s.callTrace = nil
}

// trace adds the caller's name to s.callTrace.
func (s *StateMachine) trace() {
	pc, _, _, _ := runtime.Caller(1)
	s.callTrace = append(s.callTrace, fScrub(runtime.FuncForPC(pc).Name()))
}

type logging struct {
  msgs []string
}

func (l *logging) Log(s string, i ...interface{}) {
  l.msgs = append(l.msgs, fmt.Sprintf(s, i...))
}

func TestExecutor(t *testing.T) {
	tests := []struct {
		desc string
		err  bool
    shouldLog bool
    log []string
	}{
		{
			desc: "With error in state machine execution",
			err: true,
		},
		{
			desc: "Success",
      shouldLog: true,
      log: []string{
        "StateMachine[tester]: StateFn(Start) starting",
        "StateMachine[tester]: StateFn(Start) finished",
        "StateMachine[tester]: StateFn(Middle) starting",
        "StateMachine[tester]: StateFn(Middle) finished",
        "StateMachine[tester]: StateFn(End) starting",
        "StateMachine[tester]: StateFn(End) finished",
        "StateMachine[tester]: Execute() completed with no issues",
        "StateMachine[tester]: The following is the StateFn's called with this execution:",
        "StateMachine[tester]: \tStart",
        "StateMachine[tester]: \tMiddle",
        "StateMachine[tester]: \tEnd",
      },
		},
	}

	sm := &StateMachine{}
	for _, test := range tests {
    sm.err = test.err
    l := &logging{}
		exec := New("tester", sm.Start, Reset(sm.reset), LogFacility(l.Log))
    if test.shouldLog {
      exec.Log(true)
    }else{
      exec.Log(false)
    }

		err := exec.Execute()
		switch {
		case err == nil && test.err:
			t.Errorf("Test %q: got err == nil, want err != nil", test.desc)
			continue
		case err != nil && !test.err:
			t.Errorf("Test %q: got err != %q, want err == nil", test.desc, err)
			continue
		}

		if diff := pretty.Diff(sm.callTrace, exec.Nodes()); len(diff) != 0 {
			t.Errorf("Test %q: node trace was no accurate got/want diff:\n%s", test.desc, strings.Join(diff, "\n"))
		}

    if diff := pretty.Diff(l.msgs, test.log); len(diff) != 0 {
      t.Errorf("Test %q: log was not as expected:\n%s", test.desc, strings.Join(diff, "\n"))
    }
	}
}

func TestMock(t *testing.T) {
  var _ Executor = &MockExecutor{}
}
