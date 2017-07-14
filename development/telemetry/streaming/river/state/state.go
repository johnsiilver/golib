package state

import (
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/boutique/example/notifier/state/modifiers"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
)

// NewVarState provides a boutique.Store for a variable being stored.
func NewVarState() *boutique.Store {
	state, err := boutique.New(data.VarState{}, modifiers.All, nil)
	if err != nil {
		panic(err) // the package is broken if this happens.
	}
	return state
}
