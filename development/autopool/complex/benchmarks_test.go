package complex

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	pb3 "github.com/johnsiilver/golib/development/autopool/complex/protov3"
)

const (
	hundredThousand = 100000
	oneMillion      = hundredThousand * 10
	tenMillion      = oneMillion * 10
)

func BenchmarkWithPoolGRPCv3(b *testing.B) {
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
		p.Add(reflect.TypeOf(&pb3.Output{}))

		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				GRPCBenchmark(b, p, bm.buffSize, bm.numClients, bm.numRequests)
			}
		})
	}
}

func BenchmarkWithoutPoolGRPCv3(b *testing.B) {
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
				GRPCBenchmark(b, nil, bm.buffSize, bm.numClients, bm.numRequests)
			}
		})
	}
}

func GRPCBenchmark(b *testing.B, pool *Pool, buffSize, numClients, numRequests int) {
	b.ReportAllocs()
	runtime.GC()

	serv := newGRPC(pool, buffSize)
	serv.start()
	defer serv.stop()

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

	client := pb3.NewRecorderClient(conn)
	in := make(chan *pb3.Input, 1)
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
		in <- &pb3.Input{User: "jdoe", RescUri: "/path/to/resource"}
	}
	close(in)

	wg.Wait()
}

type grpcService struct {
	p    string
	serv *grpc.Server
	lis  net.Listener

	pool *Pool

	counter  uint64
	buffSize int
}

func newGRPC(pool *Pool, buffSize int) *grpcService {
	return &grpcService{
		p:    filepath.Join(os.TempDir(), uuid.New().String()),
		pool: pool,
	}
}

func (g *grpcService) start() {
	lis, err := net.Listen("unix", g.p)
	if err != nil {
		panic(err)
	}
	server := grpc.NewServer()
	pb3.RegisterRecorderServer(server, g)
	go server.Serve(lis)
	g.lis = lis
	g.serv = server
}

func (g *grpcService) stop() {
	g.serv.Stop()
}

func (g *grpcService) id() uint64 {
	v := atomic.AddUint64(&g.counter, 1)
	return v - 1
}

var (
	outType   = reflect.TypeOf(&pb3.Output{})
	userType  = reflect.TypeOf(&pb3.User{})
	rescType  = reflect.TypeOf(&pb3.Resource{})
	permsType = reflect.TypeOf(&pb3.Perms{})
)

func (g *grpcService) Record(ctx context.Context, in *pb3.Input) (*pb3.Output, error) {
	id := g.id()

	var out *pb3.Output
	if g.pool != nil {
		out = g.pool.Get(outType).(*pb3.Output)
		out.User = g.pool.Get(userType).(*pb3.User)
		out.Resc = g.pool.Get(rescType).(*pb3.Resource)
		out.Perms = g.pool.Get(permsType).(*pb3.Perms)

		out.Id = int64(id)
		out.User.First = "John"
		out.User.Last = "Doe"
		out.User.Id = 37
		out.Resc.Type = "file"
		out.Resc.Uri = in.RescUri
		out.Resc.Payload = make([]byte, g.buffSize)
	} else {
		out = &pb3.Output{
			Id: int64(id),
			User: &pb3.User{
				First: "John",
				Last:  "Doe",
				Id:    37,
			},
			Resc: &pb3.Resource{
				Type:    "file",
				Uri:     in.RescUri,
				Payload: make([]byte, g.buffSize),
			},
		}
	}

	// Makes sure the payload gets allocated.
	if g.buffSize > 0 {
		out.Resc.Payload[0] = 1
	}

	return out, nil
}
