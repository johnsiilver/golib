package autopool

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
)

// Pool provides a sync.Pool where GC'd objects are put back into the Pool automatically.
type Pool struct {
	typeToInt map[reflect.Type]int
	pools     []*sync.Pool
	mu        sync.Mutex
}

// New creates
func New() *Pool {
	return &Pool{pools: []*sync.Pool{}, typeToInt: map[reflect.Type]int{}}
}

// Add adds support for the type t. t must be a pointer to a struct.
// Calling this after the first call to Get() or Put() is a race condition.
func (p *Pool) Add(t reflect.Type) int {
	switch t.Kind() {
	case reflect.Ptr:
	default:
		panic(fmt.Sprintf("cannot create a Pool on %v value", t))
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if v, ok := p.typeToInt[t]; ok {
		return v
	}

	p.typeToInt[t] = len(p.pools)
	p.pools = append(
		p.pools,
		&sync.Pool{
			New: func() interface{} {
				return reflect.New(t.Elem()).Interface()
			},
		},
	)
	return p.typeToInt[t]
}

func (p *Pool) IntLookup(t reflect.Type) int {
	if v, ok := p.typeToInt[t]; ok {
		return v
	}
	return -1
}

// Get retrieves a value from a pool that has the type reflect.Type. This will panic if
// that type is an unsupported type or was not a sub type of the value that was provided by the
// function passed to New().
func (p *Pool) Get(i int) interface{} {
	pool := p.pools[i]

	x := pool.Get()
	runtime.SetFinalizer(x, func(o interface{}) { p.put(i, o) })

	return x
}

// put adds x to the pool.
func (p *Pool) put(i int, x interface{}) {
	runtime.SetFinalizer(x, nil)
	p.pools[i].Put(x)
}
