package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/proto/rpc"

	pb "github.com/johnsiilver/golib/ipc/uds/highlevel/proto/rpc/example/proto"
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
	client, err := rpc.New(*addr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 1770})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := pb.QuoteResp{}
	if err := client.Call(ctx, "quote", &pb.QuoteReq{}, &resp); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Quote: ", resp.Quote)
}
