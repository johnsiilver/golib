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
	rMu      sync.Mutex   // Protects everything below.
	monitors atomic.Value // map[string]*monitorData
)

type monitorData struct {
	transport.RiverTransport

	name string
	die  chan struct{}
	opCh chan transport.Operation

	sync.RWMutex
	loopers map[string]*loopers
}

func newMonitorData(name string, trans transport.RiverTransport) *monitorData {
	m := &monitorData{
		name:           name,
		die:            make(chan struct{}),
		opCh:           make(chan transport.Operation, 1),
		loopers:        map[string]*loopers{},
		RiverTransport: trans,
	}
	go m.startSend()
	go m.startReceive()
	return m
}

// monitorWriter writes to a monitor when it receives an update for a variable.
func (md *monitorData) monitorWriter(results chan data.VarState) {
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

// starts starts our loop for listening to the monitor.
func (md *monitorData) startReceive() {
	for {
		select {
		case <-md.die:
			glog.Infof("startReceive(%s) was told to die", md.name)
			return
		case op := <-md.Receive():
			if err := op.Validate(); err != nil {
				select {
				case op.Response <- fmt.Errorf("error: monitor %s: invalid subscribeOp: %s", md.name, err):
				default:
				}
				continue
			}
			md.opCh <- op
		}
	}
}

func (md *monitorData) startSend() {
	results := make(chan data.VarState, 100)
	defer close(results)

	go md.monitorWriter(results)

	const (
		sentDie = 0
		sentOp  = 1
	)

	cases := []reflect.SelectCase{
		// #0: <-md.die
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(md.die),
		},
		// #1: <-md.opCh
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(md.opCh),
		},
	}

	for {
		chosen, val, recvOk := reflect.Select(cases)
		if !recvOk {
			glog.Infof("send to monitor %s stopped, chose %d, val %#+v", md.name, chosen, val)
			return
		}
		switch chosen {
		case sentDie:
			glog.Infof("send to monitor %s stopped, closedChan %v", md.name, recvOk)
			return
		case sentOp:
			op := val.Interface().(transport.Operation)

			switch op.Type {
			case transport.SubscribeOp:
				expVar := getRegistry()[op.Subscribe.Name]
				if expVar == nil {
					op.Response <- fmt.Errorf("monitor %v asked to monitor %s, which isn't registered", md.name, op.Subscribe.Name)
					continue
				}
				if err := handleSubscribes(md, op.Subscribe.Name, expVar, op.Subscribe.Interval, results); err != nil {
					op.Response <- fmt.Errorf("monitor %v asked to monitor %s, but had error: %s", md.name, op.Subscribe.Name, err)
					continue
				}
				glog.Infof("monitor %v will begin to receive variable %s", md.name, op.Subscribe.Name)
				op.Response <- nil
			case transport.DropOp:
				md.deleteVar(op)
			}
		}
	}
}

func (md *monitorData) deleteVar(op transport.Operation) {
	md.Lock()
	defer md.Unlock()
	l := md.loopers[op.Drop.Name]
	if l == nil {
		op.Response <- fmt.Errorf("monitor %v asked drop %s, but it didn't exist", md.name, op.Drop.Name)
		return
	}
	l.i.cancel()
	delete(md.loopers, op.Drop.Name)
	op.Response <- nil
}

type loopers struct {
	//f *funcHolder
	i *interval
}

// interval is used to only send an updates at a maximum interval.  But if that
// interval expires and we had an update during the blackout, we should then send
// that data.
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
	i.results <- i.val // Send the initial value.
	go func() {
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

func getMonitors() map[string]*monitorData {
	m := monitors.Load()
	if m == nil {
		return nil
	}
	return m.(map[string]*monitorData)
}

// RegisterMonitor connects to a river monitor. This will return an error if
// we cannot connect.  Connecting will only be attempted for the lifetime of
// ctx (by default this is infinity).
func RegisterMonitor(ctx context.Context, dest string, t transport.RiverTransport) error {
	rMu.Lock()
	defer rMu.Unlock()

	m := getMonitors()
	if m == nil {
		m = map[string]*monitorData{}
	}
	if _, ok := m[dest]; ok {
		return fmt.Errorf("RegisterMonitor: dest %s already exists", dest)
	}

	if err := t.Connect(); err != nil {
		return fmt.Errorf("RegisterMonitor: dest %s cannot be connected to: %s", dest, err)
	}
	m[dest] = newMonitorData(dest, t)
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
