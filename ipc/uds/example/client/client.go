package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/johnsiilver/golib/ipc/uds"
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
		panic(err)
	}

	// Connects to the server at socketAddr that must have the file uid/gid of
	// our current user and one of the os.FileMode specified.
	client, err := uds.NewClient(*addr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 1770})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// client implements io.ReadWriteCloser and this will print to the screen
	// whatever the server sends until the connection is closed.
	io.Copy(os.Stdout, client)
}
