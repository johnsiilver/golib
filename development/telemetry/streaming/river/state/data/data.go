// Package data provides state data for boutique stores.
package data

import (
	"expvar"
)

const (
	// IntType indicates we are storing an Int.
	IntType VarType = 1
	// FloatType indicates we are storing a Float.
	FloatType VarType = 2
	// StringType indicates we are storing a String.
	StringType VarType = 3
	// MapType indicates we are storing a Map.
	MapType VarType = 4
	// FuncType indicates we are storing a func.
	FuncType VarType = 5
)

// VarType is used to indicate what type of variable is stored.
type VarType int

// IsVarType indicates this is a VarType.
func (VarType) IsVarType() bool {
	return true
}

// VarState holds state data for expvar's.
type VarState struct {
	// Type indicates the type of variable being stored.
	Type VarType
	// Int represents an int.
	Int int64
	// Float represents a float64.
	Float float64
	// Map represents a key/value lookup of expvar.Vars.
	Map map[string]expvar.Var
	// String represents a string
	String string
	// Func represents a function.
	Func expvar.Var
}
