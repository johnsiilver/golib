/*
Package rpc provides a proto RPC service.

A simple client example (based on the server below):
	client, err := New(socketAddr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 0771})
	if err != nil {
		// Do something
	}

	ctx, cancel := context.WitheTimeout(5 * time.Second)
	req := &pb.SumReq{Ints: 1, 2, 3}
	resp := &pb.SumResp{}

	retry:
		if err := client.Call(ctx, "/math/sum", req, resp); err != nil {
			if rpc.Retryable(err) {
				// Okay, so you probably should do this in a for loop, but I wanted to use
				// a label for the hell of it.
				goto retry
			}
			// Do something here, cause you have a non-retryable error.
		}
		if resp.Err != nil {
			// Do something with the internal error.
		}
		fmt.Printf("Sum of %#v = %d\n", req, resp.Sum)

A simple service works like:
	type MathServer struct{}

	func (m *MathServer) Sum(ctx context.Context, req []byte]) ([]byte, error) {
		request := &pb.SumReq{}
		if err := proto.Unmarshal(req, request); err != nil {
			return nil, rpc.Errorf(rpc.ETBadData, "request could not be unmarshalled into SumReq: %s", err)
		}

		response := &pb.SumResp{}
		for _, i := range request.Ints {
			response.Sum += i
		}
		b, err := proto.Marshal(response)
		if err != nil {
			return nil, rpc.Errorf(rpc.ETBadData, "request could not be unmarshalled into SumReq: %s", err)
		}
		return b, nil
	}

	func main() {
		cred, err := uds.Current()
		if err != nil {
			panic(err)
		}

		serv, err := NewServer("socketAddr", cred.UID.Int(), cred.GID.Int(), 0770)
		if err != nil {
			panic(err)
		}

		ms := MathServer{}

		serv.RegisterMethod("/math/sum", ms.Sum)

		if err := serv.Start(); err != nil {
			log.Fatal(err)
		}
	}

Note: The server should only return errors to clients when something goes wrong with the RPC.
This makes it predictable on the server side when the error is retryable. When the service
has an error, I recommend returning the expected response, which should have a dict containing
your custom error code and the error message. This allows your clients to decide if they
should retry a request.
*/
package rpc

import (
	"context"
	"fmt"
	"os"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/chunk"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/chunk/rpc"

	"google.golang.org/protobuf/proto"
)

// Client provides an RPC client using JSON.
type Client struct {
	rpc *rpc.Client

	pool    *chunk.Pool
	maxSize int64
}

// Option is an optional argument to New.
type Option func(c *Client)

// MaxSize is the maximum size a read message is allowed to be. If a message is larger than this, Next()
// will fail and the underlying connection will be closed.
func MaxSize(size int64) Option {
	return func(c *Client) {
		c.maxSize = size
	}
}

// SharedPool allows the use of a shared pool of buffers between Client instead of a pool per client.
// This is useful when clients are short lived and have similar message sizes. Client will panic if the
// pool does not return a *[]byte object.
func SharedPool(pool *chunk.Pool) Option {
	return func(c *Client) {
		c.pool = pool
	}
}

// New is the constructor for Client.
func New(socketAddr string, uid, gid int, fileModes []os.FileMode, options ...Option) (*Client, error) {
	client := &Client{}
	for _, o := range options {
		o(client)
	}
	if client.pool == nil {
		client.pool = chunk.NewPool(10)
	}

	cc, err := rpc.New(socketAddr, uid, gid, fileModes, rpc.MaxSize(client.maxSize), rpc.SharedPool(client.pool))
	if err != nil {
		return nil, err
	}
	client.rpc = cc

	return client, nil
}

// Close closes the underyling connection.
func (c *Client) Close() error {
	return c.rpc.Close()
}

// Call calls the RPC service. If Context timeout is not set, will default to 5 minutes.
func (c *Client) Call(ctx context.Context, method string, req, resp proto.Message) error {
	if method == "" {
		return fmt.Errorf("must pass non-empty method arge")
	}
	if req == nil {
		return fmt.Errorf("must pass non-nil req arg")
	}
	if resp == nil {
		return fmt.Errorf("must pass non-nil resp arg")
	}

	data, err := c.marshalProto(req)
	if err != nil {
		return err
	}
	defer c.pool.Put(data)

	if c.maxSize > 0 {
		if len(*data) > int(c.maxSize) {
			return fmt.Errorf("data has a size greater than your max size limit")
		}
	}

	buff := c.pool.Get()
	defer c.pool.Put(buff)

	if err := c.rpc.Call(ctx, method, *data, buff); err != nil {
		return err
	}

	if err := proto.Unmarshal(*buff, resp); err != nil {
		return rpc.Errorf(rpc.ETBadData, "could not unmarshal into response(%T):\n%s", resp, string(*buff))
	}
	return nil
}

var marshalOpts = proto.MarshalOptions{}

func (c *Client) marshalProto(m proto.Message) (*[]byte, error) {
	buff := c.pool.Get()

	b, err := marshalOpts.MarshalAppend(*buff, m)
	if err != nil {
		return nil, err
	}

	*buff = b
	return buff, nil
}

// CredFromCtx will extract the Cred from the Context object.
func CredFromCtx(ctx context.Context) uds.Cred {
	return rpc.CredFromCtx(ctx)
}

// RequestHandler will receive a Context object with a Deadline set and you can retrieve
// the calling process creds with CredFromCtx. The bytes of the request
// and resp will be the json.Marshal of the response. An error returned is a ErrServer
// that can not be retried. Generally, service errors should be in the resp and not a
// returned error. See the note in the package intro.
type RequestHandler = rpc.RequestHandler

// Server provides a proto RPC server.
type Server struct {
	socketAddr string
	rpc        *rpc.Server
	maxSize    int64
	handlers   map[string]RequestHandler
}

// NewServer is the constructor for a Server.
func NewServer(socketAddr string, uid, gid int, fileMode os.FileMode) (*Server, error) {
	serv, err := rpc.NewServer(socketAddr, uid, gid, fileMode)
	if err != nil {
		return nil, err
	}

	return &Server{
		socketAddr: socketAddr,
		rpc:        serv,
		handlers:   map[string]RequestHandler{},
	}, nil
}

// Start starts the server. This will block until the server stops.
func (s *Server) Start() error {
	return s.rpc.Start()
}

// Stop stops the server, which will stop listening for new connections. This
// should slowly kill off existing calls. Stop will return when all calls have
// completed or the context deadline is reached(or cancelled). A nil error
// indicates that all jobs were completed. Note: a Server cannot be reused.
func (s *Server) Stop(ctx context.Context) error {
	return s.rpc.Stop(ctx)
}

// RegisterMethod registers an RPC method with the server.
func (s *Server) RegisterMethod(method string, handler RequestHandler) {
	s.rpc.RegisterMethod(method, handler)
}
