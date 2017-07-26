package modifiers

import (
	"expvar"
	"testing"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/actions"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
	"github.com/kylelemons/godebug/pretty"

	"github.com/lukechampine/freeze"
)

func TestVarStateModBasicTyps(t *testing.T) {
	tests := []struct {
		desc    string
		initial *data.VarState
		action  boutique.Action
		want    data.VarState
	}{
		{
			desc:   "NameSet()",
			action: actions.NameSet("hello"),
			want:   data.VarState{Name: "hello"},
		},
		{
			desc:    "IntSet()",
			initial: &data.VarState{Int: 20},
			action:  actions.IntSet(10),
			want:    data.VarState{Type: data.IntType, Int: 10},
		},
		{
			desc:    "IntAdd()",
			initial: &data.VarState{Int: 20},
			action:  actions.IntAdd(10),
			want:    data.VarState{Type: data.IntType, Int: 30},
		},
		{
			desc:    "FloatSet()",
			initial: &data.VarState{Float: 20},
			action:  actions.FloatSet(10),
			want:    data.VarState{Type: data.FloatType, Float: 10},
		},
		{
			desc:    "FloatAdd()",
			initial: &data.VarState{Float: 20},
			action:  actions.FloatAdd(10),
			want:    data.VarState{Type: data.FloatType, Float: 30},
		},
		{
			desc:    "String()",
			initial: &data.VarState{String: "yo"},
			action:  actions.String("ho"),
			want:    data.VarState{Type: data.StringType, String: "ho"},
		},
		{
			desc:    "NoOP()",
			initial: &data.VarState{NoOp: 1},
			action:  actions.NoOp(),
			want:    data.VarState{Type: data.MapType, NoOp: 2},
		},
	}

	for _, test := range tests {
		if test.initial == nil {
			test.initial = new(data.VarState)
		}

		// This validates that we didn't mutate our map.
		d := freeze.Pointer(test.initial).(*data.VarState)

		got := VarStateMod(*d, test.action)

		if diff := pretty.Compare(test.want, got); diff != "" {
			t.Errorf("TestVarStateMod: %s:  -want/+got:\n%s", test.desc, diff)
		}
	}
}

func TestVarStateModMapType(t *testing.T) {
	d := &data.VarState{}
	// This validates that we didn't mutate our map.
	d = freeze.Pointer(d).(*data.VarState)

	// Test StoreMap
	intVar := &expvar.Int{}
	intVar.Set(1)
	state := VarStateMod(*d, actions.StoreMap("int", intVar))
	got := state.(data.VarState).Map["int"].String()
	if got != "1" {
		t.Errorf("TestVarStateModMapType: Test StoreMap(): got %v, want %v", got, "1")
	}

	// Test DeleteMap
	state = VarStateMod(*d, actions.DeleteMap("int"))
	v := state.(data.VarState).Map["int"]
	if v != nil {
		t.Errorf("TestVarStateModMapType: Test DeleteMap(): got %v, want %v", got, nil)
	}

	// Test ReplaceMap
	state = VarStateMod(*d, actions.ReplaceMap(map[string]expvar.Var{"int": intVar}))
	got = state.(data.VarState).Map["int"].String()
	if got != "1" {
		t.Errorf("TestVarStateModMapType: Test ReplaceMap(): got %v, want %v", got, "1")
	}
}
