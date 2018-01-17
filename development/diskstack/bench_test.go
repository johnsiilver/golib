package diskstack

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/beeker1121/goque"
	"github.com/golang/glog"
	"github.com/johnsiilver/golib/development/diskstack2"
)

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

func BenchmarkStacks(b *testing.B) {
	const (
		goqueDir      = "./data_dir"
		diskqueue2Dir = "./tmp_dir"
		diskqueueFile = "./stack_tmp"
	)

	os.RemoveAll(diskqueue2Dir)
	os.RemoveAll(goqueDir)
	os.Remove(diskqueueFile)
	//os.Mkdir(diskqueue2Dir, 0666)
	os.Mkdir(goqueDir, 0777)

	// Self clean on exit
	defer os.Remove(diskqueueFile)
	defer os.RemoveAll(diskqueue2Dir)
	defer os.RemoveAll(goqueDir)

	d1, err := New(diskqueueFile, []byte{})
	if err != nil {
		b.Fatalf("BenchStacks: cannot create diskstack: %s", err)
	}
	defer d1.Close()

	d2, err := diskstack2.New(diskqueue2Dir, []byte{})
	if err != nil {
		b.Fatalf("BenchStacks: cannot create diskstack2: %s", err)
	}
	defer d2.Close()

	goq, err := goque.OpenStack(goqueDir)
	if err != nil {
		b.Fatalf("BenchStacks: cannot create goque: %s", err)
	}
	goqStack := goqueWrap{goq}
	defer goqStack.Close()

	stacks := []struct {
		name  string
		stack testStack
	}{
		{"SingleFile", d1},
		//{"MultiFile", d2},
		//{"Goque", goqStack},
	}

	benches := []struct {
		name string
		data []byte
	}{
		{"byte", make([]byte, 1)},
		//{"kibibyte", make([]byte, 1024)},
		//{"mebibyte", make([]byte, 1048576)},
		//{"10 mebibyte", make([]byte, 10490000)},
	}

	for _, bench := range benches {
		for _, stack := range stacks {
			b.Run(
				fmt.Sprintf("%s-%s", stack.name, bench.name),
				func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						glog.Infof("run %d", i)
						b.StartTimer()
						readWrite(b, stack.name, stack.stack, bench.data)
					}
				},
			)
			panic("wtf!")
		}
	}
}

func readWrite(b *testing.B, name string, stack testStack, data []byte) {
	const count = 2

	rLimiter := make(chan struct{}, 10)
	wLimiter := make(chan struct{}, 10)
	wg := sync.WaitGroup{}

	wg.Add(count)
	for x := 0; x < count; x++ {
		go func() {
			wLimiter <- struct{}{}
			defer func() { <-wLimiter }()
			if err := stack.Push(data); err != nil {
				b.Fatalf("readWrite: %v", err)
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
					b.Fatalf("BenchmarkStacks(%s): %v", name, err)
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
