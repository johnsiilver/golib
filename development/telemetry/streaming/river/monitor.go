package river

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/johnsiilver/golib/development/telemetry/streaming/river/transport"
)

var (
	monitors atomic.Value // map[string]Transport
	rMu      sync.Mutex
)

func getMonitors() map[string]transport.RiverTransport {
	return monitors.Load().(map[string]transport.RiverTransport)
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
	m[dest] = t
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
