package river

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/actions"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/modifiers"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/transport"
)

type myID struct {
	app      string
	pod      int
	instance int
}

func (m myID) Attributes() []IDAttr {
	return []IDAttr{
		{
			Field: "App",
			Value: m.app,
		},
		{
			Field: "Pod",
			Value: fmt.Sprintf("%d", m.pod),
		},
		{
			Field: "Instance",
			Value: fmt.Sprintf("%d", m.instance),
		},
	}
}

func makeID(app string, pod, instance int) myID {
	return myID{app: app, pod: pod, instance: instance}
}

func TestInterval(t *testing.T) {
	d := data.VarState{Int: 1}
	store, err := boutique.New(d, modifiers.All, nil)
	if err != nil {
		panic(err)
	}

	sigCh, cancel, err := store.Subscribe("Int")
	if err != nil {
		panic(err)
	}
	defer cancel()

	start := time.Now()
	results := make(chan data.VarState, 1)
	interval := newInterval("varName", 10*time.Second, d, sigCh, cancel, results)
	interval.Go()
	select {
	case i := <-results:
		if i.Int != 1 {
			t.Fatalf("TestInterval: expected initial value: got %v, want %v", i, 1)
		}
	default:
		t.Fatalf("TestInterval: did not get initial value")
	}

	store.Perform(actions.IntAdd(1))
	store.Perform(actions.IntAdd(1))
	i := <-results
	diffTime := time.Now().Sub(start)
	if diffTime < 10*time.Second {
		t.Errorf("TestInterval: value is streaming in faster than our interval: %v", diffTime)
	}

	if i.Int != 3 {
		t.Errorf("TestInterval: final value: got %v, want %v", i.Int, 3)
	}
}

type fakeTransport struct {
	connectErr bool
	receive    chan transport.Operation
	toMonitor  chan data.VarState
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		connectErr: false,
		receive:    make(chan transport.Operation, 1),
		toMonitor:  make(chan data.VarState, 1),
	}
}

func (f *fakeTransport) Connect() error {
	if f.connectErr {
		return errors.New("connect error")
	}
	return nil
}

func (f *fakeTransport) Receive() <-chan transport.Operation {
	return f.receive
}

func (f *fakeTransport) Send(v data.VarState) error {
	select {
	case f.toMonitor <- v:
	default:
		return errors.New("Send() error")
	}
	return nil
}

func (f *fakeTransport) Close() {
	return
}

////////////////////////
// These are to allow us to pretend we are the monitor.
///////////////////////
func (f *fakeTransport) Operation(op transport.Operation) error {
	f.receive <- op
	return <-op.Response
}

func TestMonitorData(t *testing.T) {
	const interval = 5 * time.Second

	SetID(makeID("fakeApp", 0, 0))

	myInt := MakeInt(0)
	myString := MakeString("")
	Publish("int", myInt)
	Publish("string", myString)
	defer registry.Store(map[string]expvar.Var{})

	trans := newFakeTransport()

	RegisterMonitor(context.Background(), "fakeMonitor0", trans)
	defer monitors.Store(map[string]*monitorData{})

	var (
		monitorInt = new(int64)
		monitorStr = atomic.Value{}
		monitorDie = make(chan struct{})
	)
	monitorStr.Store("")

	defer close(monitorDie)

	go func() {
		for {
			select {
			case <-monitorDie:
				return
			case vs := <-trans.toMonitor:
				switch vs.Name {
				case "int":
					glog.Infof("monitor received 'int' update")
					atomic.CompareAndSwapInt64(monitorInt, *monitorInt, vs.Int)
				case "string":
					glog.Infof("monitor received 'string' update: %s", vs.String)
					monitorStr.Store(vs.String)
				default:
					panic(fmt.Sprintf("received unknown variable to monitor: %s", vs.Name))
				}
			}
		}
	}()

	err := trans.Operation(
		transport.Operation{
			Type:      transport.SubscribeOp,
			Subscribe: transport.Subscribe{Name: "int", Interval: interval},
			Response:  make(chan error, 1),
		},
	)
	if err != nil {
		t.Fatalf("TestMonitorData: unexpected error: %s", err)
	}

	err = trans.Operation(
		transport.Operation{
			Type:      transport.SubscribeOp,
			Subscribe: transport.Subscribe{Name: "string", Interval: interval},
			Response:  make(chan error, 1),
		},
	)
	if err != nil {
		t.Fatalf("TestMonitorData: unexpected error: %s", err)
	}

	startTime := time.Now()
	myInt.Add(1)
	myString.Set("!ok")
	okay := make(chan struct{})
	intAt4 := make(chan struct{})

	go func() {
		for {
			if monitorStr.Load().(string) == "ok" {
				close(okay)
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()
	go func() {
		for {
			if atomic.LoadInt64(monitorInt) == 4 {
				close(intAt4)
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()

	myString.Set("!ok")
	myString.Set("ok")
	<-okay
	if time.Now().Sub(startTime) < interval {
		t.Errorf("TestMonitorData: monitor data 'string' is coming in faster than our interval setting")
	}
	myInt.Add(3)
	<-intAt4

	err = trans.Operation(
		transport.Operation{
			Type:     transport.DropOp,
			Drop:     transport.Drop{Name: "string"},
			Response: make(chan error, 1),
		},
	)
	if err != nil {
		t.Fatalf("TestMonitorData: unexpected error: %s", err)
	}

	myString.Set("ouch")

	now := time.Now()
	for {
		if time.Now().Sub(now) > interval {
			break
		}
		if monitorStr.Load().(string) == "ouch" {
			t.Errorf("TestMonitorData: continued to monitor 'string' after it was told to drop it")
		}
		time.Sleep(1 * time.Second)
	}
}
