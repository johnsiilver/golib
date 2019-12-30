// Package autopool provides the ability to modify objects in a Pool so that they are
// automatically added back to the pool on GC. This allows submitting to the pool objects that
// leave your code's control, such as a protocol buffer that is gRPC will encode to the caller.
// It should be noted that this package has no control over when the GC frees the object (if ever),
// so you should explicitly call .Put() when you can.  This will not stop GC by the sync.Pool itself.
package autopool

import (
	"runtime"
	"sync"
)

// Pool provides a sync.Pool where GC'd objects are put back into the Pool automatically.
type Pool struct {
	pool *sync.Pool
}

// New modifies the existing *sync.Pool to return objects that attempt to return
// to the Pool when the GC is prepared to free them.  Only safe to use before the
// Pool is used.
func New(pool *sync.Pool) *Pool {
	return &Pool{pool}
}

// Get works the same as sync.Pool.Get() with the exception that a finalizer is set
// on the object to return the item to the Pool when it is GC'd. Note: objects passed
// that depend on finalizers should not be used here, as they are cleared at certain
// points in the objects lifetime by this package.
func (p *Pool) Get() interface{} {
	x := p.pool.Get()
	runtime.SetFinalizer(
		x,
		func(x interface{}) {
			p.Put(x)
		},
	)
	return x
}

// Put adds x to the pool.
func (p *Pool) Put(x interface{}) {
	runtime.SetFinalizer(x, nil)
	p.pool.Put(x)
}
