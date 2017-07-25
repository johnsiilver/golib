package state

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/modifiers"
)

// NewVarState provides a boutique.Store for a variable being stored.
func NewVarState(name string) *boutique.Store {
	state, err := boutique.New(data.VarState{Name: name}, modifiers.All, nil)
	if err != nil {
		panic(err) // the package is broken if this happens.
	}
	return state
}
