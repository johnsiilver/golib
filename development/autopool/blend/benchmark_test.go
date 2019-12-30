package blend

import (
	"context"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	pb "github.com/johnsiilver/golib/development/autopool/blend/proto"

	_ "net/http/pprof"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
}

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
		/*
			{"100 Clients/1K Buffer/100K Requests", 100, 1024, hundredThousand},
			{"100 Clients/1K Buffer/1M Requests", 100, 1024, oneMillion},
			{"100 Clients/8K Buffer/1M Requests", 100, 1024 * 8, oneMillion},
			{"100 Clients/64K Buffer/1M Requests", 100, 1024 * 64, oneMillion},
			{"100 Clients/128K Buffer/1M Requests", 100, 1024 * 128, oneMillion},
			{"100 Clients/512K Buffer/1M Requests", 100, 1024 * 512, oneMillion},
			{"100 Clients/1M Buffer/1M Requests", 100, 1024 * 1024, oneMillion},
		*/
		{"100 Clients/10M Buffer/1M Requests", 100, 1024 * 1024 * 10, oneMillion},
		//{"100 Clients/1K Buffer/10M Requests", 100, 1024, tenMillion},
	}

	p := New()
	p.Add(func() interface{} { return &pb.Output{} })   // Returns 0, but we are just going to statically use it.
	p.Add(func() interface{} { return &pb.Resource{} }) // Returns 1, but we are just going to statically use it.

	for _, bm := range benches {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				GRPCBenchmark(b, p, bm.buffSize, bm.numClients, bm.numRequests)
			}
		})
		log.Printf("%d/%d(attempts/misses)", p.attempts, p.misses)
	}
}

func BenchmarkWithoutPoolGRPCv3(b *testing.B) {
	benches := []struct {
		name        string
		numClients  int
		buffSize    int
		numRequests int
	}{
		/*
			{"100 Clients/1K Buffer/100K Requests", 100, 1024, hundredThousand},
			{"100 Clients/1K Buffer/1M Requests", 100, 1024, oneMillion},
			{"100 Clients/8K Buffer/1M Requests", 100, 1024 * 8, oneMillion},
			{"100 Clients/64K Buffer/1M Requests", 100, 1024 * 64, oneMillion},
			{"100 Clients/128K Buffer/1M Requests", 100, 1024 * 128, oneMillion},
			{"100 Clients/512K Buffer/1M Requests", 100, 1024 * 512, oneMillion},
			{"100 Clients/1M Buffer/1M Requests", 100, 1024 * 1024, oneMillion},
		*/
		{"100 Clients/10M Buffer/1M Requests", 100, 1024 * 1024 * 10, oneMillion},
		//{"100 Clients/1K Buffer/10M Requests", 100, 1024, tenMillion},
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

	client := pb.NewRecorderClient(conn)
	in := make(chan *pb.Input, 1)
	wg := sync.WaitGroup{}
	ctx := context.Background()
	outList := []*pb.Output{}

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for req := range in {
				out, err := client.Record(ctx, req)
				if err != nil {
					panic(err)
				}
				outList = append(outList, out)
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < numRequests; i++ {
		in <- &pb.Input{User: "jdoe", RescUri: "/path/to/resource"}
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
	pb.RegisterRecorderServer(server, g)
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

var spool = &sync.Pool{
	New: func() interface{} {
		return &pb.Resource{}
	},
}

func (g *grpcService) Record(ctx context.Context, in *pb.Input) (*pb.Output, error) {
	id := g.id()

	var out *pb.Output
	if g.pool != nil {
		out = &pb.Output{}

		out.Id = int64(id)
		out.User = &pb.User{
			First: "John",
			Last:  "Doe",
			Id:    37,
		}
		//out.Resc = g.pool.Get(1).(*pb.Resource)
		out.Resc = spool.Get().(*pb.Resource)
		//runtime.SetFinalizer(out.Resc, func(o *pb.Resource) { spool.Put(o) })
		out.Resc.Type = "file"
		out.Resc.Uri = in.RescUri
		out.Resc.Payload = out.Resc.Payload[0:0]
	} else {
		out = &pb.Output{
			Id: int64(id),
			User: &pb.User{
				First: "John",
				Last:  "Doe",
				Id:    37,
			},
			Resc: &pb.Resource{
				Type:    "file",
				Uri:     in.RescUri,
				Payload: []byte{},
			},
		}
	}

	for i := 0; i < g.buffSize; i++ {
		out.Resc.Payload = append(out.Resc.Payload, byte(rand.Int31()))
	}
	spool.Put(out.Resc)

	return out, nil
}
