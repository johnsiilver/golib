package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/proto/rpc"
	"google.golang.org/protobuf/proto"

	pb "github.com/johnsiilver/golib/ipc/uds/proto/rpc/benchmark/proto"
)

var reqPool = &sync.Pool{
	New: func() interface{} {
		return &pb.BenchmarkReq{}
	},
}

var respPool = &sync.Pool{
	New: func() interface{} {
		return &pb.BenchmarkResp{}
	},
}

func requestHandler(ctx context.Context, req []byte) ([]byte, error) {
	in := reqPool.Get().(*pb.BenchmarkReq)
	defer func() {
		in.Reset()
		reqPool.Put(in)
	}()

	if err := proto.Unmarshal(req, in); err != nil {
		return nil, err
	}

	resp := respPool.Get().(*pb.BenchmarkResp)
	defer func() {
		resp.Reset()
		respPool.Put(resp)
	}()
	resp.Data = in.Data
	return proto.Marshal(resp)
}

func newUDS() (socketAddr string, serv *rpc.Server) {
	socketAddr = filepath.Join(os.TempDir(), uuid.New().String())

	cred, _, err := uds.Current()
	if err != nil {
		panic(err)
	}

	// Setup server.
	serv, err = rpc.NewServer(socketAddr, cred.UID.Int(), cred.GID.Int(), 0770)
	if err != nil {
		panic(err)
	}
	serv.RegisterMethod("benchmark", requestHandler)

	return socketAddr, serv
}

func udsTest(concurrency, nRequests, dataSize int) *Stats {
	socketAddr, serv := newUDS()

	go func() {
		log.Println("started")
		if err := serv.Start(); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Second)

	defer func() {
		if err := serv.Stop(context.Background()); err != nil {
			panic(err)
		}
		log.Println("stopped")
	}()

	cred, _, err := uds.Current()
	if err != nil {
		panic(err)
	}

	client, err := rpc.New(socketAddr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 0771})
	if err != nil {
		panic(err)
	}

	// Setup our caller func that will be used in our pool.
	data := make([]byte, dataSize)
	ctx := context.Background()
	var caller call = func() benchmarks {
		b := benchmarks{}
		b.startTime = time.Now()

		req := &pb.BenchmarkReq{Data: data}
		resp := &pb.BenchmarkResp{}
		if err := client.Call(ctx, "benchmark", req, resp); err != nil {
			panic(err)
		}
		b.endTime = time.Now()
		return b
	}

	pool := newPool(concurrency)
	start := time.Now()
	for i := 0; i < nRequests; i++ {
		pool.in <- caller
	}
	close(pool.in)
	pool.wg.Wait()
	end := time.Now()

	return newStats(concurrency, dataSize, pool.records, start, end)
}
