package complex

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
)

// Pool provides a sync.Pool where GC'd objects are put back into the Pool automatically.
type Pool struct {
	attempts, misses int32

	pools map[reflect.Type]*sync.Pool
	mu    sync.Mutex
}

// New creates
func New() *Pool {

	return &Pool{pools: map[reflect.Type]*sync.Pool{}}
}

// Add adds support for the type returned by f(). f() must create a new instance of a pointer to a struct, map or slice value.
// Calling this after the first call to Get() or Put() is a race condition.
func (p *Pool) Add(t reflect.Type) {
	switch t.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice:
	default:
		panic(fmt.Sprintf("cannot create a Pool on %v value", t))
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bp := buildPools{m: p.pools, misses: &p.misses}
	bp.walk(t)
}

// Get retrieves a value from a pool that has the type reflect.Type. This will panic if
// that type is an unsupported type or was not a sub type of the value that was provided by the
// function passed to New().
func (p *Pool) Get(t reflect.Type) interface{} {
	atomic.AddInt32(&p.attempts, 1)

	pool := p.pools[t]
	if pool == nil {
		panic(fmt.Sprintf("type %v is not valid type to retrieve from the pool", t))
	}

	x := pool.Get()
	/*
		runtime.SetFinalizer(
			x,
			func(x interface{}) {
				p.Put(x)
			},
		)
	*/
	p.finalizeSubObjs(reflect.ValueOf(x))
	return x
}

// Put adds x to the pool.
func (p *Pool) Put(x interface{}) {
	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Invalid {
		return
	}
	if v.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("cannot store a non-pointer value: %s", v.Kind()))
	}

	if v.IsNil() {
		return
	}

	if v.Elem().Kind() == reflect.Struct {
		prepPtrStruct(v)
	}

	runtime.SetFinalizer(x, nil)
	pool := p.pools[v.Type()]
	if pool == nil {
		panic(fmt.Sprintf("type %v is not contained in the object passed during the New() call", v.Type()))
	}

	pool.Put(x)
}

var (
	stringType     = reflect.TypeOf(new(string))
	boolType       = reflect.TypeOf(new(bool))
	intType        = reflect.TypeOf(new(int))
	int8Type       = reflect.TypeOf(new(int8))
	int16Type      = reflect.TypeOf(new(int16))
	int32Type      = reflect.TypeOf(new(int32))
	int64Type      = reflect.TypeOf(new(int32))
	uintType       = reflect.TypeOf(new(uint))
	uint8Type      = reflect.TypeOf(new(uint8))
	uint16Type     = reflect.TypeOf(new(uint16))
	uint32Type     = reflect.TypeOf(new(uint32))
	uint64Type     = reflect.TypeOf(new(int32))
	float32Type    = reflect.TypeOf(new(float32))
	float64Type    = reflect.TypeOf(new(float64))
	complex64Type  = reflect.TypeOf(new(complex64))
	complex128Type = reflect.TypeOf(new(complex128))
)

func (p *Pool) Bool(b bool) *bool {
	v := p.pools[boolType]
	var ptr *bool
	if v != nil {
		ptr = v.Get().(*bool)

	} else {
		ptr = new(bool)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = b
	return ptr
}

func (p *Pool) String(s string) *string {
	v := p.pools[stringType]
	var ptr *string
	if v != nil {
		ptr = v.Get().(*string)

	} else {
		ptr = new(string)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = s
	return ptr
}

func (p *Pool) Int(i int) *int {
	v := p.pools[intType]
	var ptr *int
	if v != nil {
		ptr = v.Get().(*int)

	} else {
		ptr = new(int)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = i
	return ptr
}

func (p *Pool) Int8(i int8) *int8 {
	v := p.pools[int8Type]
	var ptr *int8
	if v != nil {
		ptr = v.Get().(*int8)

	} else {
		ptr = new(int8)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = i
	return ptr
}

func (p *Pool) Int16(i int16) *int16 {
	v := p.pools[int16Type]
	var ptr *int16
	if v != nil {
		ptr = v.Get().(*int16)

	} else {
		ptr = new(int16)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = i
	return ptr
}

func (p *Pool) Int32(i int32) *int32 {
	v := p.pools[int32Type]
	var ptr *int32
	if v != nil {
		ptr = v.Get().(*int32)

	} else {
		ptr = new(int32)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = i
	return ptr
}

func (p *Pool) Int64(i int64) *int64 {
	v := p.pools[int64Type]
	var ptr *int64
	if v != nil {
		ptr = v.Get().(*int64)

	} else {
		ptr = new(int64)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = i
	return ptr
}

func (p *Pool) Uint(u uint) *uint {
	v := p.pools[uintType]
	var ptr *uint
	if v != nil {
		ptr = v.Get().(*uint)

	} else {
		ptr = new(uint)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = u
	return ptr
}

func (p *Pool) Uint8(u uint8) *uint8 {
	v := p.pools[uint8Type]
	var ptr *uint8
	if v != nil {
		ptr = v.Get().(*uint8)

	} else {
		ptr = new(uint8)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = u
	return ptr
}

func (p *Pool) Uint16(u uint16) *uint16 {
	v := p.pools[uint16Type]
	var ptr *uint16
	if v != nil {
		ptr = v.Get().(*uint16)

	} else {
		ptr = new(uint16)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = u
	return ptr
}

func (p *Pool) Uint32(u uint32) *uint32 {
	v := p.pools[uint32Type]
	var ptr *uint32
	if v != nil {
		ptr = v.Get().(*uint32)

	} else {
		ptr = new(uint32)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = u
	return ptr
}

func (p *Pool) Uint64(u uint64) *uint64 {
	v := p.pools[uint64Type]
	var ptr *uint64
	if v != nil {
		ptr = v.Get().(*uint64)

	} else {
		ptr = new(uint64)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = u
	return ptr
}

func (p *Pool) Float32(f float32) *float32 {
	v := p.pools[int32Type]
	var ptr *float32
	if v != nil {
		ptr = v.Get().(*float32)

	} else {
		ptr = new(float32)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = f
	return ptr
}

func (p *Pool) Float64(f float64) *float64 {
	v := p.pools[int64Type]
	var ptr *float64
	if v != nil {
		ptr = v.Get().(*float64)

	} else {
		ptr = new(float64)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = f
	return ptr
}

func (p *Pool) Complex64(c complex64) *complex64 {
	v := p.pools[complex64Type]
	var ptr *complex64
	if v != nil {
		ptr = v.Get().(*complex64)

	} else {
		ptr = new(complex64)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = c
	return ptr
}

func (p *Pool) Complex128(c complex128) *complex128 {
	v := p.pools[complex128Type]
	var ptr *complex128
	if v != nil {
		ptr = v.Get().(*complex128)

	} else {
		ptr = new(complex128)
		atomic.AddInt32(&p.misses, 1)
	}

	*ptr = c
	return ptr
}

func (p *Pool) finalizeSubObjs(v reflect.Value) {
	if v.Kind() != reflect.Ptr {
		return
	}

	switch v.Type().Elem().Kind() {
	case reflect.Struct:
		runtime.SetFinalizer(v.Interface(), nil)
		runtime.SetFinalizer(v.Interface(), func(o interface{}) { p.Put(o) })
		//p.finalizeStructFields(v)
	case reflect.Slice, reflect.Map:
		runtime.SetFinalizer(v.Interface(), nil)
		runtime.SetFinalizer(v.Interface(), func(o interface{}) { p.Put(o) })
	default:
		runtime.SetFinalizer(v.Interface(), nil)
		runtime.SetFinalizer(v.Interface(), func(o interface{}) { p.Put(o) })
	}
}

/*
func (p *Pool) finalizeStructFields(x reflect.Value) {
	for i := 0; i < x.Elem().NumField(); i++ {
		if x.Elem().Field(i).Kind() == reflect.Ptr {
			runtime.SetFinalizer(x.Elem().Field(i).Interface(), nil)
			runtime.SetFinalizer(x.Elem().Field(i).Interface(), func(o interface{}) { p.Put(o) })
		}
	}
}
*/

type buildPools struct {
	misses *int32

	m map[reflect.Type]*sync.Pool
}

func (b *buildPools) walk(t reflect.Type) {
	b.discover(t)
}

func (b *buildPools) discover(t reflect.Type) {
	if _, ok := b.m[t]; ok {
		return
	}

	switch t.Kind() {
	case reflect.Ptr:
		switch t.Elem().Kind() {
		case reflect.Struct:
			b.ptrStruct(t)
		case reflect.Slice:
			b.ptrSlice(t)
		case reflect.Map:
			b.ptrMap(t)
		case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint,
			reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
			b.ptrBasic(t)
		}
	case reflect.Struct:
		b.hstruct(t)
	case reflect.Slice:
		b.hslice(t)
	case reflect.Map:
		b.hmap(t)
	}
}

func (b *buildPools) hstruct(t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		b.discover(t.Field(i).Type)
	}
}

func (b *buildPools) hslice(t reflect.Type) {
	if t.Elem().Kind() != reflect.Ptr {
		return
	}

	b.m[t] = &sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(b.misses, 1)
			return reflect.New(t.Elem()).Interface()
		},
	}
	b.discover(t.Elem())
}

func (b *buildPools) hmap(t reflect.Type) {
	if t.Key().Kind() == reflect.Interface {
		return
	}
	if t.Elem().Kind() != reflect.Ptr {
		return
	}

	b.m[t.Elem()] = &sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(b.misses, 1)
			return reflect.New(t.Elem()).Interface()
		},
	}

	b.discover(t.Elem())
}

func (b *buildPools) ptrStruct(t reflect.Type) {
	b.m[t] = &sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(b.misses, 1)
			return reflect.New(t.Elem()).Interface()
		},
	}
	b.discover(t.Elem())
}

func (b *buildPools) ptrSlice(t reflect.Type) {
	if t.Elem().Kind() == reflect.Interface {
		return
	}

	b.m[t] = &sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(b.misses, 1)
			ptr := reflect.New(t.Elem())
			return ptr.Interface()
		},
	}
	b.discover(t.Elem())
}

func (b *buildPools) ptrMap(t reflect.Type) {
	if t.Elem().Key().Kind() == reflect.Interface {
		return
	}
	if t.Elem().Elem().Kind() == reflect.Interface {
		return
	}

	b.m[t] = &sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(b.misses, 1)
			ptr := reflect.New(t.Elem())
			return ptr.Interface()
		},
	}
	b.discover(t.Elem())
}

func (b *buildPools) ptrBasic(t reflect.Type) {
	b.m[t] = &sync.Pool{
		New: func() interface{} {
			atomic.AddInt32(b.misses, 1)
			ptr := reflect.New(t.Elem())
			return ptr.Interface()
		},
	}
}

func prepPtrStruct(x reflect.Value) {
	e := x.Elem()
	for i := 0; i < e.NumField(); i++ {
		f := e.Field(i)
		if f.Kind() == reflect.Ptr && f.Elem().Kind() == reflect.Struct {
			f.Set(reflect.Zero(f.Type()))
		}
	}
}
