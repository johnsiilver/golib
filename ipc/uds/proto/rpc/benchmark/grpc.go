package main

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	pb "github.com/johnsiilver/golib/ipc/uds/proto/rpc/benchmark/proto"
)

type server struct{}

func (s *server) Benchmark(ctx context.Context, req *pb.BenchmarkReq) (*pb.BenchmarkResp, error) {
	in := &pb.BenchmarkReq{}
	return &pb.BenchmarkResp{Data: in.Data}, nil
}

func newGRPC() (socketAddr string, serv *grpc.Server) {
	socketAddr = filepath.Join(os.TempDir(), uuid.New().String())

	myServ := &server{}

	var opts []grpc.ServerOption
	serv = grpc.NewServer(opts...)
	pb.RegisterBenchmarkServer(serv, myServ)

	return socketAddr, serv
}

func grpcTest(concurrency, nRequests, dataSize int) *Stats {
	socketAddr, serv := newGRPC()

	lis, err := net.Listen("unix", socketAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	go func() {
		serv.Serve(lis)
	}()
	time.Sleep(1 * time.Second)

	defer func() {
		serv.Stop()
		log.Println("grpc stopped")
	}()

	conn, err := grpc.Dial(
		socketAddr,
		grpc.WithInsecure(),
		grpc.WithDialer(
			func(addr string, timeout time.Duration) (net.Conn, error) {
				return net.Dial("unix", addr)
			},
		),
	)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	defer conn.Close()

	client := pb.NewBenchmarkClient(conn)

	// Setup our caller func that will be used in our pool.
	data := make([]byte, dataSize)
	ctx := context.Background()
	var caller call = func() benchmarks {
		b := benchmarks{}
		b.startTime = time.Now()

		req := &pb.BenchmarkReq{Data: data}
		_, err := client.Benchmark(ctx, req)
		if err != nil {
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
