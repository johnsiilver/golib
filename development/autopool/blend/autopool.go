/*
Package autopool provides a collection of sync.Pool(s) that holds values that are collected when
they go out of scope vs. a standard sync.Pool that requires a .Put(). This is useful when the value
goes out of scope in a package you do not control, like gRPC. If the value is under your control,
a standard sync.Pool is superior in every way. Use when the struct holds a map or slice that
is sizeable (> 50KiB). Also, YOU MUST KNOW that the third-party package does not hold a reference
to any of the fields when the pointer's value is garbage collected. Either look at the code or
use --race to verify.

Usage is simple:
 	...
 	// Create our pool and assign it to "s", which represents a gRPC service.
 	s.pool = New()
 	s.outputID = p.Add(
 		func() interface{} {
 			&pb.Output{
 				Payload: make([]byte, 100), // Presizing when I need to create new ones because the pool is empty.
 			},
 		},
 	)
 	...

 	// Use our pool to construct out Output and allow reclaining after gRPC finishes with our Output object.
 	func (s *service) Call(ctx context.Context, in *pb.Input) (*pb.Output, error) {
 		output := s.pool.Get(s.outputID).(*pb.Output)
 		resetOutput(output)

 		...
 		return output, nil
 	}
 	...

 	// resetOutput prepates our object for reuse by setting the zero values and making our Payload
 	// have a 0 len (but not a 0 capacity). If your pointer to a struct is for a Protocol Buffer,
 	// you cannot use the built in .Reset(), as this destroys the buffers.
 	func resetOutput(o *pb.Output) {
 		o.Id = 0
 		o.User = ""
 		o.Payload = o.Payload[0:0]
 	}
*/
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

// New is the constructor for Pool.
func New() *Pool {
	return &Pool{pools: []*sync.Pool{}, typeToInt: map[reflect.Type]int{}}
}

// Add adds support for type returned by f(). This must be a pointer to a struct.
// Calling this after the first call to Get() or Put() is a race condition.
func (p *Pool) Add(f func() interface{}) int {
	i := f()
	t := reflect.TypeOf(i)

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
			New: f,
		},
	)
	return p.typeToInt[t]
}

// IntLookup will return the id used in Get() to fetch a value of this type. Returns -1 if the
// Pool does not suppor the type.
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
	p.pools[i].Put(x)
}
