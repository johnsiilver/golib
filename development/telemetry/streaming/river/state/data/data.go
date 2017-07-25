// Package data provides state data for boutique stores.
package data

import (
	"expvar"
	"fmt"
)

const (
	// UnknownType indicates that the VarType wasn't set.
	UnknownType VarType = 0
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

// String implements fmt.Stringer.
func (v VarType) String() string {
	switch v {
	case IntType:
		return "int64"
	case FloatType:
		return "float64"
	case StringType:
		return "string"
	case MapType:
		return "Map"
	case FuncType:
		return "Func"
	case UnknownType:
		return "unknown"
	default:
		panic("VarType is of a type that was set, but we don't support (error in VarType.String())")
	}
}

// SubString returns the string needed to Subscribe to this variable's changes.
func (v VarType) SubString() (string, error) {
	switch v {
	case IntType:
		return "Int", nil
	case FloatType:
		return "Float", nil
	case StringType:
		return "String", nil
	case MapType:
		return "Map", nil
	case FuncType:
		return "", fmt.Errorf("Func type found, can't subscribe directly to a Func")
	case UnknownType:
		return "", fmt.Errorf("unknown type found, can't subscribe")
	default:
		panic("VarType is of a type that was set, but we don't support (error in VarType.Sub())")
	}
}

// VarState holds state data for expvar's.
type VarState struct {
	// Name is the name of the published variable this represents.
	Name string
	// Type indicates the type of variable being stored.
	Type VarType
	// Int represents an int.
	Int int64
	// Float represents a float64.
	Float float64
	// Map represents a key/value lookup of expvar.Vars.
	Map map[string]expvar.Var
	// NoOp is incremented to indicate a Map sub value has changed.
	NoOp uint64
	// String represents a string
	String string
	// Func represents a function.
	Func func() interface{}
}

// Value returns the internally held value.
func (v VarState) Value() interface{} {
	switch v.Type {
	case IntType:
		return v.Int
	case FloatType:
		return v.Float
	case MapType:
		return v.Map
	case StringType:
		return v.String
	case FuncType:
		return v.Func
	default:
		return nil
	}
}

// ValueType returns the ValueType held in VarState.
func (v VarState) ValueType() VarType {
	return v.Type
}
