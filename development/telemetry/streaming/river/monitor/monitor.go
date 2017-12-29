package monitor

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/transport"
)

type subHandle struct {
	// v is the name of a variable.
	v string
	// h is the Handler that will handle the variable v.
	h Handler
}

type Muxer struct {
	trans transport.MonitorTransport
	ids   map[string]*Identity

	mu     sync.Mutex
	subReg map[string][]subHandle
}

func NewMuxer(t transport.MonitorTransport) {

}

func (m *Muxer) Register(id string, v string, h Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sl, ok := m.subReg[id]; ok {
		// TODO(johnsiilver): fill this in.
	} else {
		m.subReg[id] = []subHandle{{v: v, h: h}}
	}
}

func (m *Muxer) Serve() {
	for in := range m.trans.Receive() {
		id := m.ids[in.ID]

		// We haven't seen this subscriber before.
		if id == nil {
			m.mu.Lock()
			subs := m.subReg[in.ID]
			m.mu.Unlock()
			if subs == nil {
				glog.Infof("identifier %q is sending data, but we have no Registered subscriptions, dropping", in.ID)
				continue
			}
			var err error
			id = NewIdentity(in.ID, m.trans)
			m.ids[in.ID] = id
			for _, sub := range subs {
				id.Subscribe(sub.v, sub.h)
			}
		}

		// Handle sending the data to the Identity.
	}
}

func (m *Muxer) Sub()

// Handler handles a change to a subscribed variable. Only one Handler is
// guarenteed to be firing for a given variable.
type Handler func(name string, v transport.Var)

// Identity provides the ability to handle incoming monitoring variables from
// an application with a specific Identity.
type Identity struct {
	id    string
	trans transport.MonitorTransport
	subMu sync.Mutex

	varChans atomic.Value // map[string]chan transport.Var
}

// NewIdentity is the constructor for Identity.
func NewIdentity(id string, t transport.MonitorTransport) *Identity {
	i := &Identity{
		id:    id,
		trans: t,
	}
	i.varChans.Store(map[string]chan transport.Var{})
	return i
}

func (s *Identity) getChan(n string) chan transport.Var {
	return s.varChans.Load().(map[string]chan transport.Var)[n]
}

func (s *Identity) setChan(n string) {
	m := s.varChans.Load().(map[string]chan transport.Var)
	m[n] = make(chan transport.Var, 100)
	s.varChans.Store(m)
}

func (s *Identity) rmChan(n string) {
	m := s.varChans.Load().(map[string]chan transport.Var)
	ch := m[n]
	if ch != nil {
		close(ch)
	}
	delete(m, n)
	s.varChans.Store(m)
}

func (s *Identity) Subscribe(n string, h Handler) error {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	if n == "" {
		return fmt.Errorf("cannot subscribe to empty string")
	}

	if h == nil {
		return fmt.Errorf("cannot provide a nil handler")
	}

	if h := s.getChan(n); h != nil {
		return fmt.Errorf("subscriber %s is already subscribed", n)
	}

	s.setChan(n)
	go s.handle(n, h, s.getChan(n))

	if err := s.trans.Subscribe(s.id, n); err != nil {
		s.rmChan(n)
		return err
	}

	return nil
}

func (s *Identity) handle(n string, h Handler, ch chan transport.Var) {
	for v := range ch {
		h(n, v)
	}
}

// Drop tells the service to stop updating us about the variable and then
// stops all handlers for that variable.
func (s *Identity) Drop(n string) error {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	if err := s.trans.Drop(s.id, n); err != nil {
		return err
	}

	s.rmChan(n)
	return nil
}

func (s *Identity) receiver() {
	for v := range s.trans.Receive(s.id) {
		ch := s.getChan(v.Name)
		if ch == nil {
			glog.Errorf("the server is streaming us variable %s, but we have not asked for it", v.Name)
			continue
		}
		ch <- v
	}
}
