package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/json/stream"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/json/stream/example/messages"

	"github.com/tjarratt/babble"
)

var (
	addr = flag.String("addr", "", "The path to the unix socket to dial")
)

func main() {
	flag.Parse()

	if *addr == "" {
		fmt.Println("did not pass --addr")
		os.Exit(1)
	}

	cred, _, err := uds.Current()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Connects to the server at socketAddr that must have the file uid/gid of
	// our current user and one of the os.FileMode specified.
	client, err := uds.NewClient(*addr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 1770})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	streamer, err := stream.New(client)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	babbler := babble.NewBabbler()

	// Client writes.
	go func() {
		defer wg.Done()

		for {
			m := messages.Word{Word: babbler.Babble()}
			if err := streamer.Write(m); err != nil {
				if err != io.EOF {
					log.Println(err)
				}
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Client reads.
	go func() {
		defer wg.Done()

		sum := messages.Sum{}
		for {
			if err := streamer.Read(&sum); err != nil {
				if err != io.EOF {
					log.Println(err)
				}
				return
			}
			fmt.Printf("Sum message: %s\n", strings.Join(sum.Sum, " "))
		}
	}()

	wg.Wait()
}
