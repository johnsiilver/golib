package autopool

import (
	"bytes"
	"context"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	pb "github.com/johnsiilver/golib/development/autopool/proto"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

const (
	oneMillion = 1000000
	numStreams = 1000
	buffSize   = 1024 * 10
)

type pooler interface {
	Get() interface{}
	Put(interface{})
}

type serviceStream struct {
	in       chan *bytes.Buffer
	out      chan *bytes.Buffer
	outOnce  sync.Once
	doneOnce sync.Once

	done    chan struct{}
	pool    pooler
	putPool bool
}

func newServiceStream(in chan *bytes.Buffer, pool pooler, putPooler bool) *serviceStream {
	s := serviceStream{
		in:      in,
		out:     make(chan *bytes.Buffer, 1),
		done:    make(chan struct{}),
		pool:    pool,
		putPool: putPooler,
	}

	go s.inToOut()
	go s.outToNull()

	return &s
}

func (s *serviceStream) inToOut() {
	defer close(s.out)

	for v := range s.in {
		s.out <- v
	}
}

func (s *serviceStream) outToNull() {
	defer close(s.done)

	for v := range s.out {
		v.Reset()
		if s.putPool {
			s.pool.Put(v)
		}
	}
}

func BenchmarkWithoutPool(b *testing.B) {
	b.ReportAllocs()

	in := make(chan *bytes.Buffer, 1)
	streams := make([]*serviceStream, numStreams)

	for i := 0; i < numStreams; i++ {
		stream := newServiceStream(in, nil, false)
		streams[i] = stream
	}

	b.ResetTimer()
	for i := 0; i < oneMillion; i++ {
		buff := bytes.NewBuffer(make([]byte, buffSize))
		for x := 0; x < buffSize; x++ {
			buff.WriteByte(byte(rand.Int31()))
		}
		in <- buff
	}
	close(in)

	for _, stream := range streams {
		<-stream.done
	}
}

func BenchmarkWithAutopoolWithPut(b *testing.B) {
	b.ReportAllocs()

	var pool = New(
		&sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, buffSize))
			},
		},
	)

	in := make(chan *bytes.Buffer, 1)
	streams := make([]*serviceStream, numStreams)

	for i := 0; i < numStreams; i++ {
		stream := newServiceStream(in, pool, true)
		streams[i] = stream
	}

	b.ResetTimer()
	for i := 0; i < oneMillion; i++ {
		buff := pool.Get().(*bytes.Buffer)
		for x := 0; x < buffSize; x++ {
			buff.WriteByte(byte(rand.Int31()))
		}
		in <- buff
	}
	close(in)

	for _, stream := range streams {
		<-stream.done
	}
}

func BenchmarkWithAutopoolWithoutPut(b *testing.B) {
	b.ReportAllocs()

	var pool = New(
		&sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, buffSize))
			},
		},
	)

	in := make(chan *bytes.Buffer, 1)
	streams := make([]*serviceStream, numStreams)

	for i := 0; i < numStreams; i++ {
		stream := newServiceStream(in, pool, true)
		streams[i] = stream
	}

	b.ResetTimer()
	for i := 0; i < oneMillion; i++ {
		buff := pool.Get().(*bytes.Buffer)
		for x := 0; x < buffSize; x++ {
			buff.WriteByte(byte(rand.Int31()))
		}
		in <- buff
	}
	close(in)

	for _, stream := range streams {
		<-stream.done
	}

}

func BenchmarkWithStandardPool(b *testing.B) {
	b.ReportAllocs()

	var pool = &sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, buffSize))
		},
	}

	in := make(chan *bytes.Buffer, 1)
	streams := make([]*serviceStream, numStreams)

	for i := 0; i < numStreams; i++ {
		stream := newServiceStream(in, pool, true)
		streams[i] = stream
	}

	b.ResetTimer()
	for i := 0; i < oneMillion; i++ {
		buff := pool.Get().(*bytes.Buffer)
		for x := 0; x < buffSize; x++ {
			buff.WriteByte(byte(rand.Int31()))
		}
		in <- buff
	}
	close(in)

	for _, stream := range streams {
		<-stream.done
	}
}

func BenchmarkStandardGRPC(b *testing.B) {
	b.ReportAllocs()

	serv := newGRPC(nil)
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

	for i := 0; i < 1000; i++ {
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

	for i := 0; i < oneMillion; i++ {
		in <- &pb.Input{First: "John", Last: "Doe"}
	}
	close(in)

	wg.Wait()
}

func BenchmarkWithOuputPoolGRPC(b *testing.B) {
	b.ReportAllocs()

	serv := newGRPC(
		New(
			&sync.Pool{
				New: func() interface{} {
					return &pb.Output{}
				},
			},
		),
	)
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

	for i := 0; i < 1000; i++ {
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

	for i := 0; i < oneMillion; i++ {
		in <- &pb.Input{First: "John", Last: "Doe"}
	}
	close(in)

	wg.Wait()
}

type rec struct {
	id          string
	first, last string
}

type grpcService struct {
	p    string
	serv *grpc.Server
	lis  net.Listener

	pool *Pool

	counter uint64

	mu  sync.Mutex
	rec map[uint64]rec
}

func newGRPC(pool *Pool) *grpcService {
	return &grpcService{
		p:    filepath.Join(os.TempDir(), uuid.New().String()),
		rec:  map[uint64]rec{},
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

func (g *grpcService) Record(ctx context.Context, in *pb.Input) (*pb.Output, error) {
	id := g.id()

	g.mu.Lock()
	defer g.mu.Unlock()

	var out *pb.Output
	if g.pool != nil {
		out = g.pool.Get().(*pb.Output)
	} else {
		out = &pb.Output{Payload: make([]byte, buffSize)}
	}

	g.rec[id] = rec{first: in.First, last: in.Last}
	out.Id = id
	rand.Read(out.Payload)
	return out, nil
}
