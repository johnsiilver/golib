// Package actions provides boutique.Actions to signal modifications to the store.
package actions

import (
	"expvar"

	"github.com/johnsiilver/boutique"
)

const (
	// ActIntSet indicates we are going to change the VarState.Int to a specific number.
	ActIntSet boutique.ActionType = iota
	// ActIntAdd indicates we are going to change the VarState.Int by adding
	// to its existing value.
	ActIntAdd
	// ActFloatSet indicates we are going to change the VarState.Float to a specific number.
	ActFloatSet
	// ActFloatadd indicates we are going to change the VarState.Float by adding
	// to its existing value.
	ActFloatAdd
	// ActString indicates we are going to change the VarState.String .
	ActString
	// ActMapStore indicates we are adding a key/value to the VarState.Map .
	ActMapStore
	// ActMapDelete indicates we are removing a key/value from the VarState.Map .
	ActMapDelete
	// ActMapReplace indicates we are replacing the VarState.Map .
	ActMapReplace
)

// IntSet produces an Action that will change the VarState.Int field by setting
// its value.
func IntSet(i int64) boutique.Action {
	return boutique.Action{Type: ActIntSet, Update: i}
}

// IntAdd produces an Action that will change its value by adding to its value.
func IntAdd(i int64) boutique.Action {
	return boutique.Action{Type: ActIntAdd, Update: i}
}

// FloatSet produces an Action that will change the VarState.Float field by
// setting its value.
func FloatSet(f float64) boutique.Action {
	return boutique.Action{Type: ActFloatSet, Update: f}
}

// FloatAdd produces an Action that will change the VarState.Float field by
// adding to its value.
func FloatAdd(f float64) boutique.Action {
	return boutique.Action{Type: ActFloatAdd, Update: f}
}

// String produces an Action that will change the VarState.String field.
func String(s string) boutique.Action {
	return boutique.Action{Type: ActString, Update: s}
}

// StoreMap produces an Action that will store a key/value pair in the
// VarState.Map field. If the key exists, it will be changed to the new
// value.
func StoreMap(k string, v expvar.Var) boutique.Action {
	return boutique.Action{Type: ActMapStore, Update: expvar.KeyValue{Key: k, Value: v}}
}

// DeleteMap produces an Action that will delete a key/value pair in the
// VarState.Map field.
func DeleteMap(k string) boutique.Action {
	return boutique.Action{Type: ActMapDelete, Update: k}
}

// ReplaceMap produces an Action that will replace the map in the
// VarState.Map field.
func ReplaceMap(m map[string]expvar.Var) boutique.Action {
	return boutique.Action{Type: ActMapReplace, Update: m}
}
