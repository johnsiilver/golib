package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/proto/rpc"

	"google.golang.org/protobuf/proto"

	pb "github.com/johnsiilver/golib/ipc/uds/highlevel/proto/rpc/example/proto"
)

var quotes = []string{
	"The greatest glory in living lies not in never falling, but in rising every time we fall. -Nelson Mandela",
	"The way to get started is to quit talking and begin doing.  -Walt Disney",
	"Your time is limited, so don't waste it living someone else's life. Don't be trapped by dogma – which is living with the results of other people's thinking. -Steve Jobs",
	"If life were predictable it would cease to be life, and be without flavor. -Eleanor Roosevelt",
	"If you look at what you have in life, you'll always have more. If you look at what you don't have in life, you'll never have enough. -Oprah Winfrey",
	"If you set your goals ridiculously high and it's a failure, you will fail above everyone else's success. -James Cameron",
	"Life is what happens when you're busy making other plans. -John Lennon",
}

func main() {
	socketAddr := filepath.Join(os.TempDir(), uuid.New().String())

	cred, _, err := uds.Current()
	if err != nil {
		panic(err)
	}

	serv, err := rpc.NewServer(socketAddr, cred.UID.Int(), cred.GID.Int(), 0770)
	if err != nil {
		panic(err)
	}

	fmt.Println("Listening on socket: ", socketAddr)

	// We can reuse our requests to get better allocation performance.
	reqPool := sync.Pool{
		New: func() interface{} {
			return &pb.QuoteReq{}
		},
	}
	// We can reuse our responses to get better allocation performance.
	respPool := sync.Pool{
		New: func() interface{} {
			return &pb.QuoteResp{}
		},
	}

	// Register a method to handle calls for "quote". I did this inline, normally
	// you do this in its own func block.
	serv.RegisterMethod(
		"quote",
		func(ctx context.Context, req []byte) (resp []byte, err error) {
			reqpb := reqPool.Get().(*pb.QuoteReq)
			defer func() {
				reqpb.Reset()
				reqPool.Put(reqpb)
			}()

			// Get the request.
			if err := proto.Unmarshal(req, reqpb); err != nil {
				return nil, err
			}

			resppb := respPool.Get().(*pb.QuoteResp)
			defer func() {
				resppb.Reset()
				respPool.Put(resppb)
			}()

			resppb.Quote = quotes[rand.Intn(len(quotes))]
			return proto.Marshal(resppb)
		},
	)

	// This blocks until the server stops.
	serv.Start()
}
