// Package modifiers provides modifiers for all boutique.Store changes.
package modifiers

import (
	"expvar"

	"github.com/golang/glog"
	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/actions"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
)

// All provides for all boutique.Modifiers used in the boutique.Store.
var All = boutique.NewModifiers(VarStateMod)

// VarStateMod provides all modifications needed for data.VarState.
func VarStateMod(state interface{}, action boutique.Action) interface{} {
	s := state.(data.VarState)

	switch action.Type {
	case actions.ActIntSet:
		s.Type = data.IntType
		s.Int = action.Update.(int64)
	case actions.ActIntAdd:
		s.Type = data.IntType
		s.Int = s.Int + action.Update.(int64)
	case actions.ActFloatSet:
		s.Type = data.FloatType
		s.Float = action.Update.(float64)
	case actions.ActFloatAdd:
		s.Type = data.FloatType
		s.Float = s.Float + action.Update.(float64)
	case actions.ActString:
		s.Type = data.StringType
		s.String = action.Update.(string)
	case actions.ActMapStore:
		s.Type = data.MapType
		kv := action.Update.(expvar.KeyValue)
		n := make(map[string]expvar.Var, len(s.Map))
		mapCopy(s.Map, n)
		n[kv.Key] = kv.Value
		s.Map = n
	case actions.ActMapDelete:
		s.Type = data.MapType
		k := action.Update.(string)
		n := make(map[string]expvar.Var, len(s.Map))
		mapCopy(s.Map, n)
		delete(n, k)
		s.Map = n
	default:
		glog.Errorf("VarStateMod does not understand the type of action: %v", action.Type)
	}
	return s
}

func mapCopy(from, to map[string]expvar.Var) {
	for k, v := range from {
		to[k] = v
	}
}
