package main

import (
	"os"
	"sync"
	"time"

	"github.com/beeker1121/goque"
	"github.com/golang/glog"
	"github.com/johnsiilver/golib/development/diskstack"
	"github.com/johnsiilver/golib/development/diskstack2"
	"github.com/spf13/pflag"
)

var (
	size   = pflag.String("size", "", "The size of the test: kb, mb, 10mb")
	count  = pflag.Int("count", 1000, "The amount of entries to do, defaults to 1000")
	method = pflag.String("method", "", "The stack method to use")
)

var methods = map[string]testStack{} // Set by init().

const (
	goqueDir      = "./data_dir"
	diskstack2Dir = "./tmp_dir"
	diskstackFile = "./stack_tmp"
)

func init() {
	d1, err := diskstack.New(diskstackFile, []byte{}, diskstack.NoFlush())
	if err != nil {
		glog.Fatalf("BenchStacks: cannot create diskstack: %s", err)
	}
	methods["diskstack"] = d1

	d2, err := diskstack2.New(diskstack2Dir, []byte{})
	if err != nil {
		glog.Fatalf("BenchStacks: cannot create diskstack2: %s", err)
	}
	methods["diskstack2"] = d2

	goq, err := goque.OpenStack(goqueDir)
	if err != nil {
		glog.Fatalf("BenchStacks: cannot create goque: %s", err)
	}
	goqStack := goqueWrap{goq}
	methods["goque"] = goqStack
}

func main() {
	pflag.Parse()

	var bSize int
	switch *size {
	case "kb":
		bSize = 1024
	case "mb":
		bSize = 1048576
	case "10mb":
		bSize = 10490000
	default:
		panic("--size incorrect")
	}

	stack, ok := methods[*method]
	if !ok {
		panic("--method incorrect")
	}

	os.RemoveAll(diskstack2Dir)
	os.RemoveAll(goqueDir)
	os.Remove(diskstackFile)
	os.Mkdir(goqueDir, 0777)
	os.Mkdir(diskstack2Dir, 0777)

	// Self clean on exit
	defer os.Remove(diskstackFile)
	defer os.RemoveAll(diskstack2Dir)
	defer os.RemoveAll(goqueDir)

	readWrite(stack, *count, make([]byte, bSize))
}

type testStack interface {
	Pop(data interface{}) (ok bool, err error)
	Push(data interface{}) error
	Close() error
}

type goqueWrap struct {
	stack *goque.Stack
}

func (g goqueWrap) Pop(data interface{}) (ok bool, err error) {
	d := data.(*[]byte)
	item, err := g.stack.Pop()
	if err != nil {
		if err == goque.ErrEmpty {
			return false, nil
		}
		return false, err
	}
	var b []byte
	if err = item.ToObject(&b); err != nil {
		return false, err
	}
	*d = b
	return true, nil
}

func (g goqueWrap) Push(data interface{}) error {
	if _, err := g.stack.PushObject(data); err != nil {
		return err
	}
	return nil
}

func (g goqueWrap) Close() error {
	return g.stack.Close()
}

func readWrite(stack testStack, count int, data []byte) {
	rLimiter := make(chan struct{}, 10)
	wLimiter := make(chan struct{}, 10)
	wg := sync.WaitGroup{}

	wg.Add(count)
	for x := 0; x < count; x++ {
		go func() {
			wLimiter <- struct{}{}
			defer func() { <-wLimiter }()
			if err := stack.Push(data); err != nil {
				glog.Fatalf("readWrite: %v", err)
			}
		}()

		go func() {
			defer wg.Done()
			var data []byte

			rLimiter <- struct{}{}
			defer func() { <-rLimiter }()

			for {
				ok, err := stack.Pop(&data)
				if err != nil {
					glog.Fatalf("BenchmarkStacks: %v", err)
				}

				if !ok {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				return
			}
		}()
	}

	wg.Wait()
}
