package main

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

func main() {
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		socketAddr := filepath.Join(os.TempDir(), uuid.New().String())
		l, err := net.Listen("unix", socketAddr)
		if err != nil {
			panic(err)
		}

		wg.Add(1)
		go func() {
			conn, _ := l.Accept()
			wg.Done()
			time.Sleep(5 * time.Minute)
			conn.Close()
		}()

		wg.Add(1)
		go func() {
			conn, _ := net.Dial("unix", socketAddr)
			wg.Done()
			time.Sleep(5 * time.Minute)
			conn.Close()
		}()
	}

	wg.Wait()
}
