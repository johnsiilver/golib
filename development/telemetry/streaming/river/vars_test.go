package river

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnsiilver/boutique"
	"github.com/johnsiilver/golib/development/telemetry/streaming/river/state/data"
	"github.com/kylelemons/godebug/pretty"
)

func init() {
	SetID(makeID("fakeApp", 0, 0))
}

// TestRiverVar makes sure that all our types conform to river.Var.
func TestRiverVar(t *testing.T) {
	_ = []Var{
		Int{}, Float{}, String{}, &Map{},
	}
}

func TestInt(t *testing.T) {
	t.Parallel()

	x := MakeInt(1)

	// Subscribe to the changes.
	sub, cancel := x.subscribe()
	defer cancel()
	var final atomic.Value // data.VarState

	// Have something listening to the changes.
	go func() {
		for i := range sub {
			final.Store(i.State.Data.(data.VarState))
			time.Sleep(1 * time.Second) // Pause to make sure we miss some changes.
		}
	}()

	// Add 1000 to our number, but do it with 1000 goroutines.
	wg := sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x.Add(1)
		}()
	}
	wg.Wait() // Wait for this to finish.

	// Make sure we get a final update of 1001, but wait no more than 10 seconds.
	start := time.Now()
	for {
		f := final.Load().(data.VarState)
		if time.Now().Sub(start) > 10*time.Second {
			t.Fatalf("TestInt: final subscribe int: got %v, want %v", f.Int, 1001)
		}
		if f.Int == 1001 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if x.String() != "1001" {
		t.Errorf("TestInt: Int.String(): got %v, want %v", x.String(), "1001")
	}

	if x.Value() != 1001 {
		t.Errorf("TestInt: Int.Value(): got %v, want %v", x.Value(), 1001)
	}

	if diff := pretty.Compare(data.VarState{Name: "unnamed", Type: data.IntType, Int: 1001}, x.varState()); diff != "" {
		t.Errorf("TestInt: Int.varState(): -want/+got:\n%s", diff)
	}

	x.Set(10)
	if x.Value() != 10 {
		t.Errorf("TestInt: Int.Set(10): got %v, want %v", x.Value(), 10)
	}
}

func TestFloat(t *testing.T) {
	t.Parallel()

	x := MakeFloat(1)

	// Subscribe to the changes.
	sub, cancel := x.subscribe()
	defer cancel()
	var final atomic.Value // data.VarState

	// Have something listening to the changes.
	go func() {
		for i := range sub {
			final.Store(i.State.Data.(data.VarState))
			time.Sleep(1 * time.Second) // Pause to make sure we miss some changes.
		}
	}()

	// Add 1000 to our number, but do it with 1000 goroutines.
	wg := sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x.Add(1)
		}()
	}
	wg.Wait() // Wait for this to finish.

	// Make sure we get a final update of 1001, but wait no more than 10 seconds.
	start := time.Now()
	for {
		f := final.Load().(data.VarState)
		if time.Now().Sub(start) > 10*time.Second {
			t.Fatalf("TestFloat: final subscribe float: got %v, want %v", f.Float, 1001)
		}
		if f.Float == 1001 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if x.String() != "1001" {
		t.Errorf("TestFloat: Flaot.String(): got %v, want %v", x.String(), "1001")
	}

	if x.Value() != 1001 {
		t.Errorf("TestFloat: Float.Value(): got %v, want %v", x.Value(), 1001)
	}

	if diff := pretty.Compare(data.VarState{Name: "unknown", Type: data.FloatType, Float: 1001}, x.varState()); diff != "" {
		t.Errorf("TestFloat: Float.varState(): -want/+got:\n%s", diff)
	}

	x.Set(10)
	if x.Value() != 10 {
		t.Errorf("TestFloat: Float.Set(10): got %v, want %v", x.Value(), 10)
	}
}

func TestString(t *testing.T) {
	t.Parallel()

	x := MakeString("b")

	// Subscribe to the changes.
	sub, cancel := x.subscribe()
	defer cancel()
	var final atomic.Value // data.VarState

	// Have something listening to the changes.
	go func() {
		for i := range sub {
			final.Store(i.State.Data.(data.VarState))
			time.Sleep(1 * time.Second) // Pause to make sure we miss some changes.
		}
	}()

	str := []string{"b"}
	for i := 0; i < 1000; i++ {
		str = append(str, "a")
		x.Set(strings.Join(str, ""))
	}
	finalStr := strings.Join(str, "")

	start := time.Now()
	for {
		f := final.Load().(data.VarState)
		if time.Now().Sub(start) > 10*time.Second {
			t.Fatalf("TestString: final subscribe string: got %v, want %v", f.String, finalStr)
		}
		if f.String == finalStr {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	b, _ := json.Marshal(finalStr)
	if x.String() != string(b) {
		t.Errorf("TestString: String.String(): got %v, want %v", x.String(), string(b))
	}

	if x.Value() != finalStr {
		t.Errorf("TestString: String.Value(): got %v, want %v", x.Value(), finalStr)
	}

	if diff := pretty.Compare(data.VarState{Name: "unknown", Type: data.StringType, String: finalStr}, x.varState()); diff != "" {
		t.Errorf("TestString: String.varState(): -want/+got:\n%s", diff)
	}
}

func TestMap(t *testing.T) {
	t.Parallel()

	x := MakeMap()

	// Subscribe to the changes.
	sub, cancel := x.subscribe()
	defer cancel()
	var final atomic.Value // data.VarState

	// Have something listening to the changes.
	go func() {
		for i := range sub {
			final.Store(i.State.Data.(data.VarState))
			time.Sleep(1 * time.Second) // Pause to make sure we miss some changes.
		}
	}()

	// Test Set(), Add(), AddFloat().
	wg := sync.WaitGroup{}
	x.Set("int", MakeInt(0))
	for i := 0; i < 1000; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			x.Add("int", 1)
		}()
		go func() {
			defer wg.Done()
			x.AddFloat("float", 1)
		}()
	}
	wg.Wait()

	if x.Get("int").String() != "1000" {
		t.Errorf("TestMap: key 'int': got %s, want %s", x.Get("int"), "1000")
	}

	if x.Get("float").String() != "1000" {
		t.Errorf("TestMap: key 'float': got %v, want %v", x.Get("float"), "1000")
	}

	// Test that updating a var in a map gives us an update.
	i := MakeInt(200)
	x.Set("great", i)
	up, cancel := x.subscribe()
	defer cancel()
	var sig boutique.Signal
	wg.Add(1)
	ready := make(chan struct{})
	go func() {
		defer wg.Done()
		// Clear out any update we have waiting.
		select {
		case <-up:
		default:
		}
		close(ready)
		sig = <-up
	}()

	<-ready

	i.Add(1)
	wg.Wait()

	got := sig.State.Data.(data.VarState).Map["great"].String()
	if got != "201" {
		t.Errorf("TestMap: testing Map.Set() subscriptions: got %v, want %v", got, "201")
	}

	wantMap := map[string]interface{}{
		"int":   1000,
		"float": 1000.0,
		"great": 201,
	}

	gotMap := map[string]interface{}{}
	if err := json.Unmarshal([]byte(x.String()), &gotMap); err != nil {
		t.Fatalf("TestMap: Map.String() json.Unmarshal: %s", err)
	}

	if diff := pretty.Compare(wantMap, gotMap); diff != "" {
		t.Errorf("TestMap: testing Map.String(): -want/+got:\n%s", diff)
	}

	x.Add("addInt", 3)
	if x.Get("addInt").String() != "3" {
		t.Errorf("TestMap: calling .Add() on non-existent key does not work")
	}

	if x.Init() != x {
		t.Errorf("TestMap: calling .Init() should return the same map")
	}
}

func TestNew(t *testing.T) {
	defer func() {
		registry = atomic.Value{}
	}()

	tests := []struct {
		desc    string
		name    string
		varType string
		init    func(n string)
	}{
		{
			desc:    "NewInt()",
			name:    "int",
			varType: "river.Int",
			init: func(n string) {
				NewInt(n)
			},
		},
		{
			desc:    "NewFloat()",
			name:    "float",
			varType: "river.Float",
			init: func(n string) {
				NewFloat(n)
			},
		},
		{
			desc:    "NewString()",
			name:    "string",
			varType: "river.String",
			init: func(n string) {
				NewString(n)
			},
		},
		{
			desc:    "NewMap()",
			name:    "map",
			varType: "*river.Map",
			init: func(n string) {
				NewMap(n)
			},
		},
	}

	for _, test := range tests {
		test.init(test.name)
		reg := getRegistry()
		if v, ok := reg[test.name]; !ok {
			t.Errorf("TestNew: %s: could not find registry entry after New[Type]() call", test.desc)
			continue
		} else {
			gotType := fmt.Sprintf("%T", v)
			if gotType != test.varType {
				t.Errorf("TestNew: %s: registered wrong type, got %v, want %v", test.desc, gotType, test.varType)
			}
		}
	}
}
