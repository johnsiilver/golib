package transport

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
)

// OpType is the type of operation we are attempting to do.
type OpType int

// IsOpType is used to indicate this type is Optype.
func (OpType) IsOpType() {}

const (
	// UnknownOp indicates we don't know what type of Operation this is.
	// This is always a bug.
	UnknownOp OpType = 0
	// SubscribeOp indicates we are trying to subscribe to certain variables.
	SubscribeOp OpType = 1
	// DropOp indicates we are trying to remove certain variables from
	// monitoring.
	DropOp OpType = 2
)

// Operation is an instruction from the monitor to a river application.
type Operation struct {
	// Type is the operation type that is happening.
	Type OpType

	// Subscribe contains data if the Type === SubscribeOp.
	Subscribe Subscribe

	// Drop contains data if the Type == DropOp.
	Drop Drop

	// Response is the response to this operation. If it its nil, then it
	// succeeded. Otherwise there was a failure.
	Response chan error
}

// Validate validates the operation has the correct attributes et.
func (o Operation) Validate() error {
	switch o.Type {
	case SubscribeOp:
		return o.Subscribe.Validate()
	case DropOp:
		return o.Drop.Validate()
	default:
		panic("unknown OpType to Validate()")
	}
}

// Subscribe describes a monitors attempt to subscribe to a variable.
type Subscribe struct {
	// Name is the name of the variable to subscribe to.
	Name string
	// Interval is the minimum time between updates.
	Interval time.Duration
}

// Validate validates that the Subscribe contains data within constraints.
func (s Subscribe) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("cannot subscribe to a blank variable name")
	}
	if s.Interval > 1*time.Minute {
		return fmt.Errorf("cannot subscribe to a variable with an Interval > 1 minute")
	}
	return nil
}

// Drop describes a moniors attempt to drop a variable.
type Drop struct {
	// Name is the name of the variable to drop.
	Name string
}

// Validate validates that the Drop contains data within constraints.
func (d Drop) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("cannot drop a blank variable name")
	}
	return nil
}

// Source is the river app that is connecting to a monitor.
type Source struct {
	// App is the name of the application, as reported by the sender.
	// This may not be empty.  It may not contain ":".
	App string
	// Shard is a collection of applications sharing some type of relationship.
	// Maybe a kubernetes pod or other construct.  This may not contain ":".
	// This may be empty.
	Shard string
	// IP is the IP address that the river instance is connected from.
	IP net.IP
	// Port is the port that the river instance connected from.
	Port int32
	// Instance is a particular instance of an app on a IP:Port combination, to
	// distiguish the app from other instances. This can be empty.
	Instance int32
}

// Validate validates the Source.
func (s Source) Validate() error {
	if len(s.App) == 0 {
		return fmt.Errorf("Source.App cannot be empty")
	}
	if strings.Contains(":", s.App) {
		return fmt.Errorf("Source.App cannot contain ':'")
	}
	if strings.Contains(":", s.Shard) {
		return fmt.Errorf("Source.Shard cannot contain ':'")
	}
	if len(s.IP) == 0 {
		return fmt.Errorf("Source.IP cannot be empty")
	}
	if s.Port == 0 {
		return fmt.Errorf("Source.Port cannot be empty")
	}
	return nil
}

// String implements Stringer.
// You should not rely on this for tests as this format can change.
func (s Source) String() string {
	return fmt.Sprintf("%s:%s:%s:%d:%d", s.App, s.Shard, s.IP, s.Port, s.Instance)
}

// VarType indicates the type of variable that is being stored.
type VarType int

// IsVarType indicates that this is a VarType.
func (VarType) IsVarType() {}

const (
	// UnknownVar indicates that the program is broken if this is seen.
	UnknownVar VarType = 0
	// IntVar indicates we storing an int64.
	IntVar VarType = 1
	// FloatVar indicates we are storign a float64.
	FloatVar VarType = 2
	// StringVar indicates we are storing a string.
	StringVar VarType = 3
)

// Var is a monitored variable that is transported from a river instance to
// a monitor.
type Var struct {
	// Name is the name of the variable.
	Name string
	// Source is the source of the variable.
	Source Source
	// Type is the type of variable.
	Type VarType
	// Int is an int64.
	Int int64
	// Float is a float64.
	Float float64
	// String is a string.
	String string
}

// RiverTransport is used to implement a client for talking to a monitor over some
// type of underlying transport (websocket, grpc, etc).  All methods must be
// type safe.
type RiverTransport interface {
	// Connect connects to the remote monitor.
	Connect() error
	// Receive returns a channel that we read Operations from.
	Receive() <-chan Operation
	// Send sends a variable to the monitor.
	Send(v data.VarState) error
	// Close closes the Transport's connection.
	Close()
}

// IdentityVar is a variable sent from an Identity.
type IdentityVar struct {
	// ID is an identity for an application instance talking to a monitor.
	ID string
	// Var is the variable sent to the monitor from the Identity.
	Var Var
}

// MonitorTransport is implemented by a monitor to be able to receive data
// from a river application.
type MonitorTransport interface {
	// Listen allows for the transport to start listening for connections.
	Listen(hostPort string) error
	// Subscribe subscribes the monitor to start receiving a variable.
	Subscribe(id string, v string) error
	// Drop drops s variable from being monitored.
	Drop(id string, vars string) error
	// Receive a variable that has changed from a river instance.
	Receive() <-chan IdentityVar
	// Close closes the Transport's connection.
	Close()
}
