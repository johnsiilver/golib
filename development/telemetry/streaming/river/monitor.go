package river

import (
	"context"
	"expvar"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/transport"
)

var (
	monitors    atomic.Value // map[string]monitorData
	monitorsVer atomic.Value //
	rMu         sync.Mutex
)

type monitorData struct {
	transport.RiverTransport

	name    string
	die     chan struct{}
	addVar  chan transport.Subscribe
	dropVar chan transport.Drop

	sync.RWMutex
	loopers map[string]*loopers
}

type loopers struct {
	//f *funcHolder
	i *interval
}

type interval struct {
	name   string
	cancel boutique.CancelFunc

	// interval is the minimum wait between sending changes
	interval time.Duration
	// val is the current value we have.
	val data.VarState

	// intervalCh is used to signal a change in the interval we must adhere to.
	intervalCh chan time.Duration
	// sigCh is used to receive changes to our watched variables.
	sigCh chan boutique.Signal
	// results is used to send the next value to be sent over the wire.
	results chan<- data.VarState
	// last is the last time we sent an update.
	last time.Time
}

func newInterval(name string, d time.Duration, initVal data.VarState, sigCh chan boutique.Signal,
	cancel boutique.CancelFunc, results chan<- data.VarState) *interval {
	return &interval{
		name:       name,
		interval:   d,
		val:        initVal,
		cancel:     cancel,
		intervalCh: make(chan time.Duration, 1),
		sigCh:      sigCh,
		results:    results,
	}
}

func (i *interval) Go() {
	go func() {
		i.results <- i.val
		lastSend := time.Now()
		stopper := make(chan struct{})
		wg := sync.WaitGroup{}
		var eventualSend func(v data.VarState, wait time.Duration)

		for {
			select {
			case iv := <-i.intervalCh:
				i.interval = iv
				continue
			case sig := <-i.sigCh:
				// sig.Version == 0 indicates that the Signal is the Zero value, meaning
				// that CancelFunc has been called.
				if sig.Version == 0 {
					return
				}

				// If we have a goroutine out there to send the last update to them
				// when the interval expires, then kill it.
				if eventualSend != nil {
					close(stopper)
					wg.Wait()
					stopper = make(chan struct{})
					eventualSend = nil
				}

				i.val = sig.State.Data.(data.VarState)

				// If we are not being stopped by the minimum interval, send the result.
				// Otherwise, create a goroutine to send them this value whenever that
				// interval expires.
				if time.Now().Sub(lastSend) > i.interval {
					i.results <- i.val
					lastSend = time.Now()
				} else {
					wg.Add(1)
					eventualSend = func(v data.VarState, wait time.Duration) {
						defer wg.Done()
						select {
						case <-stopper:
							return
						case <-time.After(wait):
							i.results <- i.val
						}
					}
					go eventualSend(i.val, i.interval)
				}
			}
		}
	}()
}

func newMonitorData(name string, trans transport.RiverTransport) *monitorData {
	return &monitorData{
		name:           name,
		die:            make(chan struct{}),
		addVar:         make(chan transport.Subscribe, 1),
		dropVar:        make(chan transport.Drop, 1),
		loopers:        map[string]*loopers{},
		RiverTransport: trans,
	}
}

func getMonitors() map[string]*monitorData {
	return monitors.Load().(map[string]*monitorData)
}

// RegisterMonitor connects to a river monitor. This will return an error if
// we cannot connect.  Connecting will only be attempted for the lifetime of
// ctx (by default this is infinity).
func RegisterMonitor(ctx context.Context, dest string, t transport.RiverTransport) error {
	rMu.Lock()
	defer rMu.Unlock()

	m := getMonitors()
	if _, ok := m[dest]; ok {
		return fmt.Errorf("RegisterMonitor: dest %s already exists", dest)
	}

	if err := t.Connect(); err != nil {
		return fmt.Errorf("RegisterMonitor: dest %s cannot be connected to: %s", dest, err)
	}
	m[dest] = newMonitorData(dest, t)
	go startSend(m[dest])
	go startReceive(m[dest])
	return nil
}

// RemoveMonitor removes all monitors
func RemoveMonitor(dest string) {
	rMu.Lock()
	defer rMu.Unlock()

	m := getMonitors()

	v, ok := m[dest]
	if !ok {
		return
	}

	v.Close()
	delete(m, dest)
	monitors.Store(m)
	return
}

// starts starts our loop for listening to the monitor.
func startReceive(md *monitorData) {
	for {
		select {
		case <-md.die:
			return
		case err := <-md.Issue():
			glog.Errorf("monitor %v is having an error: %s", md.name, err)
		case op := <-md.Receive():
			switch op.Type {
			case transport.SubscribeOp:
				if err := op.Subscribe.Validate(); err != nil {
					glog.Errorf("error: monitor %s: invalid subscribeOp: %s", md.name, err)
					continue
				}
				md.addVar <- op.Subscribe
			case transport.DropOp:
				if err := op.Drop.Validate(); err != nil {
					glog.Errorf("error: monitor %s: invalid dropOp: %s", md.name, err)
					continue
				}
				md.dropVar <- op.Drop
			case transport.UnknownOp:
				glog.Errorf("error: monitor %s is using a broken implmentation, UnknownOp OpType received", md.name)
			default:
				glog.Errorf("error: monitor %s sent an OpType not supported by this server version: %v", md.name, op.Type)
			}
		}
	}
}

func startSend(md *monitorData) {
	results := make(chan data.VarState, 100)
	defer close(results)

	go monitorWriter(md, results)

	const (
		die  = 0
		add  = 1
		drop = 2
	)

	cases := []reflect.SelectCase{
		// #0: <-md.die
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(md.die),
		},
		// #1: <-md.addVar
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(md.addVar),
		},
		// #2: <-md.dropVar
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(md.dropVar),
		},
	}

	for {
		chosen, val, closedChan := reflect.Select(cases)
		if closedChan {
			glog.Infof("send to monitor %s stopped", md.name)
			return
		}
		switch chosen {
		case die:
			glog.Infof("send to monitor %s stopped", md.name)
			return
		case add:
			v := val.Interface().(transport.Subscribe)
			expVar := getRegistry()[v.Name]
			if expVar == nil {
				// TODO(johnsiilver): Really consider informing the monitor of this issue instead of just logging it.
				glog.Errorf("monitor %v asked to monitor %s, which isn't registered", md.name, v.Name)
				continue
			}
			if err := handleSubscribes(md, v.Name, expVar, v.Interval, results); err != nil {
				// TODO(johnsiilver): Really consider informing the monitor of this issue instead of just logging it.
				glog.Errorf("monitor %v asked to monitor %s, but had error: %s", md.name, v.Name, err)
				continue
			}
		case drop:
			v := val.Interface().(transport.Drop)
			// TODO(johnsiilver): Refactor into a method.
			func() {
				md.Lock()
				defer md.Unlock()
				l := md.loopers[v.Name]
				if l == nil {
					glog.Infof("monitor %v asked drop %s, but it didn't exist", md.name, v.Name)
					return
				}
				l.i.cancel()
				delete(md.loopers, v.Name)
			}()
		}
	}
}

// monitorWriter writes to a monitor when it receives an update for a variable.
func monitorWriter(md *monitorData, results chan data.VarState) {
	glog.Infof("monitor %v monitorWriter started", md.name)
	defer glog.Infof("monitor %v monitorWriter is dead", md.name)
	defer md.Close()

	errCh := make(chan error, 1)
	for {
		select {
		case r := <-results:
			go func() {
				if err := md.Send(r); err != nil {
					select { // If the channel is full, then just drop.
					case errCh <- err:
					default:
					}
					return
				}
			}()
		case err := <-errCh:
			glog.Errorf("problem writing to monitor %v: %s", md.name, err)
			glog.Errorf("killing monitor %v", md.name)
			return
		}
	}
}

func handleSubscribes(md *monitorData, name string, v expvar.Var, interval time.Duration, writerCh chan<- data.VarState) error {
	md.Lock()
	defer md.Unlock()

	if loop, ok := md.loopers[name]; ok {
		/*
			if loop.f != nil {
				loop.f.loop(interval)
			} else {
		*/
		loop.i.intervalCh <- interval
		//}
		return nil
	}

	switch t := v.(type) {
	//case Func:
	case Var:
		sigCh, cancel := t.subscribe()

		i := newInterval(name, interval, t.varState(), sigCh, cancel, writerCh)
		md.loopers[name] = &loopers{i: i}
		i.Go()
	default:
		return fmt.Errorf("the expvar.Var %q that was passed was neither a Func or implemented the subscriber interface: %T", name, v)
	}
	return nil
}
