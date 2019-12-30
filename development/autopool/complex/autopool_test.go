package complex

import (
	//"fmt"
	"reflect"
	"testing"
)

type StructA struct {
	Int         int
	String      string
	PtrBool     *bool
	PtrInt      *int
	PtrInt8     *int8
	PtrInt16    *int16
	PtrInt32    *int32
	PtrInt64    *int64
	PtrUint     *uint
	PtrUint8    *uint8
	PtrUint16   *uint16
	PtrUint32   *uint32
	PtrUint64   *uint64
	Chan        chan struct{}
	MapSimple   map[int]string
	SliceSimple []int
	MapComplex  map[int][]int
	PtrSlice    *[]int
	PtrMap      *map[int]string
	PtrStruct   *StructB
	Struct      StructB
}

type StructB struct {
	Int           int
	String        string
	PtrFloat32    *float32
	PtrFloat64    *float64
	PtrComplex64  *complex64
	PtrComplex128 *complex128
	Chan          chan struct{}
	MapSimple     map[string]int
	SliceSimple   []string
	PtrSlice      *[]string
	PtrMap        *map[string]int
}

func TestSupportedTypes(t *testing.T) {
	tests := []struct {
		desc string
		f    reflect.Type
		gets []reflect.Type
	}{
		{
			desc: "Pointer to Struct",
			f:    reflect.TypeOf(&StructA{}),
			gets: []reflect.Type{
				reflect.TypeOf(&[]int{}),
				reflect.TypeOf(&map[int]string{}),
				reflect.TypeOf(&StructB{}),
				reflect.TypeOf(&[]string{}),
				reflect.TypeOf(&map[string]int{}),
				reflect.TypeOf(new(bool)),
				reflect.TypeOf(new(int)),
				reflect.TypeOf(new(int8)),
				reflect.TypeOf(new(int16)),
				reflect.TypeOf(new(int32)),
				reflect.TypeOf(new(int64)),
				reflect.TypeOf(new(uint)),
				reflect.TypeOf(new(uint8)),
				reflect.TypeOf(new(uint16)),
				reflect.TypeOf(new(uint32)),
				reflect.TypeOf(new(uint64)),
				reflect.TypeOf(new(float32)),
				reflect.TypeOf(new(float64)),
				reflect.TypeOf(new(complex64)),
				reflect.TypeOf(new(complex128)),
			},
		},
	}

	for _, test := range tests {
		pool := New()
		pool.Add(test.f)
		for _, get := range test.gets {
			got := pool.Get(get)
			if reflect.TypeOf(got) != get {
				t.Errorf("TestSupportedTypes(%s): pool.Get(%v), got %v", test.desc, get, reflect.TypeOf(got))
			}
			if reflect.ValueOf(got).IsNil() {
				t.Errorf("TestSupportedTypes(%s): pool.Get(%v), value returned was nil", test.desc, get)
			}
		}
		pool = New()
		pool.Add(test.f)
		for _, get := range test.gets {
			pool.Put(reflect.New(get.Elem()).Interface())

			got := pool.Get(get)
			if reflect.TypeOf(got) != get {
				t.Errorf("TestSupportedTypes(%s): pool.Get(%v), got %v", test.desc, get, reflect.TypeOf(got))
			}
			if reflect.ValueOf(got).IsNil() {
				t.Errorf("TestSupportedTypes(%s): pool.Get(%v), value returned was nil", test.desc, get)
			}
		}
	}
}
