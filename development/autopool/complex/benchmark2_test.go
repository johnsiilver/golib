package complex

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc"

	pb2 "github.com/johnsiilver/golib/development/autopool/complex/protov2"
)

func BenchmarkWithPoolGRPCv2(b *testing.B) {
	benches := []struct {
		name        string
		numClients  int
		buffSize    int
		numRequests int
	}{
		{"100 Clients/1K Buffer/100K Requests", 100, 1024, hundredThousand},
		{"100 Clients/1K Buffer/1M Requests", 100, 1024, oneMillion},
		{"100 Clients/8K Buffer/1M Requests", 100, 1024 * 8, oneMillion},
		{"100 Clients/1K Buffer/10M Requests", 100, 1024, tenMillion},
	}

	for _, bm := range benches {
		p := New()
		p.Add(reflect.TypeOf(&pb2.Output{}))

		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				GRPCBenchmarkV2(b, p, bm.buffSize, bm.numClients, bm.numRequests)
			}
		})
	}
}

func BenchmarkWithoutPoolGRPCv2(b *testing.B) {
	benches := []struct {
		name        string
		numClients  int
		buffSize    int
		numRequests int
	}{
		{"100 Clients/1K Buffer/100K Requests", 100, 1024, hundredThousand},
		{"100 Clients/1K Buffer/1M Requests", 100, 1024, oneMillion},
		{"100 Clients/8K Buffer/1M Requests", 100, 1024 * 8, oneMillion},
		{"100 Clients/1K Buffer/10M Requests", 100, 1024, tenMillion},
	}

	for _, bm := range benches {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				GRPCBenchmarkV2(b, nil, bm.buffSize, bm.numClients, bm.numRequests)
			}
		})
	}
}

func GRPCBenchmarkV2(b *testing.B, pool *Pool, numClients, buffSize, numRequests int) {
	b.ReportAllocs()
	runtime.GC()

	serv := newGRPCV2(pool, buffSize)
	serv.start()
	defer serv.stop()

	if pool != nil {
		defer func() {
			log.Printf("%d attempts/%d misses", atomic.LoadInt32(&pool.attempts), atomic.LoadInt32(&pool.misses))
		}()
	}

	conn, err := grpc.Dial(
		"don't need",
		grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", serv.p, 5*time.Second)
		}),
	)
	if err != nil {
		panic(err)
	}

	client := pb2.NewRecorderClient(conn)
	in := make(chan *pb2.Input, 1)
	wg := sync.WaitGroup{}
	ctx := context.Background()

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for req := range in {
				_, err := client.Record(ctx, req)
				if err != nil {
					panic(err)
				}
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < numRequests; i++ {
		in <- &pb2.Input{User: proto.String("jdoe"), RescUri: proto.String("/path/to/resource")}
	}
	close(in)

	wg.Wait()
}

type grpcServiceV2 struct {
	p    string
	serv *grpc.Server
	lis  net.Listener

	pool *Pool

	counter  uint64
	buffSize int
}

func newGRPCV2(pool *Pool, buffSize int) *grpcServiceV2 {
	return &grpcServiceV2{
		p:    filepath.Join(os.TempDir(), uuid.New().String()),
		pool: pool,
	}
}

func (g *grpcServiceV2) start() {
	lis, err := net.Listen("unix", g.p)
	if err != nil {
		panic(err)
	}
	server := grpc.NewServer()
	pb2.RegisterRecorderServer(server, g)
	go server.Serve(lis)
	g.lis = lis
	g.serv = server
}

func (g *grpcServiceV2) stop() {
	g.serv.Stop()
}

func (g *grpcServiceV2) id() uint64 {
	v := atomic.AddUint64(&g.counter, 1)
	return v - 1
}

var (
	out2Type   = reflect.TypeOf(&pb2.Output{})
	user2Type  = reflect.TypeOf(&pb2.User{})
	resc2Type  = reflect.TypeOf(&pb2.Resource{})
	perms2Type = reflect.TypeOf(&pb2.Perms{})
)

func (g *grpcServiceV2) Record(ctx context.Context, in *pb2.Input) (*pb2.Output, error) {
	id := g.id()

	var out *pb2.Output
	if g.pool != nil {
		out = g.pool.Get(out2Type).(*pb2.Output)
		out.User = g.pool.Get(user2Type).(*pb2.User)
		out.Resc = g.pool.Get(resc2Type).(*pb2.Resource)
		out.Perms = g.pool.Get(perms2Type).(*pb2.Perms)

		out.Id = g.pool.Int64(int64(id))
		out.User.First = g.pool.String("John")
		out.User.Last = g.pool.String("Doe")
		out.User.Id = g.pool.Int64(37)
		out.Resc.Type = g.pool.String("file")
		out.Resc.Uri = g.pool.String(*in.RescUri)
		out.Resc.Payload = make([]byte, 0, g.buffSize)
	} else {
		out = &pb2.Output{
			Id: proto.Int64(int64(id)),
			User: &pb2.User{
				First: proto.String("John"),
				Last:  proto.String("Doe"),
				Id:    proto.Int64(37),
			},
			Resc: &pb2.Resource{
				Type:    proto.String("file"),
				Uri:     proto.String(*in.RescUri),
				Payload: make([]byte, 0, g.buffSize),
			},
		}
	}

	// Makes sure the payload gets allocated.
	if g.buffSize > 0 {
		out.Resc.Payload = append(out.Resc.Payload, 1)
	}

	return out, nil
}
