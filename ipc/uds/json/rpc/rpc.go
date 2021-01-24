/*
Package rpc provides a json RPC service. This should not be confused with the standards
based JSON-RPC 1.0/2.0/... This is a specific implementation over chunked data in UDS.

This package bears more resemblence to net/http than to the net/rpc/json package, at
least for the server. This could be used to build a "reflect" type of net/rpc service
or to generate a service package like gRPC does.

A simple client example (based on the server below):
	client, err := New(pathToSocket)
	if err != nil {
		// Do something
	}

	ctx, cancel := context.WitheTimeout(5 * time.Second)
	req := server.SumReq{Ints: 1, 2, 3}
	resp := server.SumResp

	retry:
		if err := client.Call(ctx, "/math/sum", req, &resp); err != nil {
			if Retryable(err) {
				// Okay, so you probably should do this in a for loop, but I wanted to use
				// a label for the hell of it.
				goto retry
			}
			// Do something here, cause you have a non-retryable error.
		}
		if resp.Err.AsError() != nil {
			// Do something with the internal error.
		}
		fmt.Printf("Sum of %#v = %d\n", req, resp.Sum)

A simple service works like:
	type Error struct {
		Msg string
	}
	func (e Error) AsError() error {
		return e
	}
	func (e Error) Error() string {
		return e.Msg
	}

	type SumReq struct {
		Ints []int
	}

	type SumResp struct {
		Sum int
		Err Error
	}

	type MathServer struct {}

	func(m *MyServer) Sum(ctx context.Context, req []byte) (resp []byte, error) {
		request := AddReq{}
		if err := json.Unmarshal(req, &request); err != nil {
			return nil, chunkRPC.Errorf(ETBadData, "request could not be unmarshalled into AddReq: %s", err)
		}

		response := AddResp{}
		for _, i := range request.Ints {
			resp.Sum += i
		}
		b, err := json.Marshal(response)
		if err != nil {
			return nil, chunkRPC.Errorf(ETBadData, "request could not be unmarshalled into AddReq: %s", err)
		}
		return b, nil
	}

	func main() {
		user, err := uds.Current()
		if err != nil {
			panic(err)
		}
		serv, err := NewServer("socketAddr", user.UID, user.GID, 0770)
		if err != nil {
			panic(err)
		}

		ms := MathServer{}

		serv.RegisterMethod("/math/sum", ms.Sum)

		if err := serv.Start(); err != nil {
			log.Fatal(err)
		}
	}

Note: The server only returns errors to clients when something goes wrong. This makes it
predictable on the server side when the error is retryable. When the service
has an error, I recommend returning the expected response, which should have a dict containing
your custom error code and the error message. This allows your clients to decide if they
should retry a request.
*/
package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/chunk/rpc"
)

// Client provides an RPC client using JSON.
type Client struct {
	chunkRPC *rpc.Client

	pool    *sync.Pool // *bytes.Buffer
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
// pool does not return a *bytes.Buffer object.
func SharedPool(pool *sync.Pool) Option {
	return func(c *Client) {
		c.pool = pool
	}
}

// New is the constructor for Client. rwc must be a *uds.Client or *uds.Conn.
func New(rwc io.ReadWriteCloser, options ...Option) (*Client, error) {
	switch rwc.(type) {
	case *uds.Client, *uds.Conn:
	default:
		return nil, fmt.Errorf("rwc was not a *uds.Client or *uds.Server, was %T", rwc)
	}

	client := &Client{}
	for _, o := range options {
		o(client)
	}

	if client.pool == nil {
		client.pool = &sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		}
	}
	cc, err := rpc.New(rwc, rpc.MaxSize(client.maxSize), rpc.SharedPool(client.pool))
	if err != nil {
		return nil, err
	}
	client.chunkRPC = cc

	return client, nil
}

// Close closes the underyling connection.
func (c *Client) Close() error {
	return c.chunkRPC.Close()
}

// Call calls the RPC service. If Context timeout is not set, will default to 5 minutes.
func (c *Client) Call(ctx context.Context, method string, req, resp interface{}) error {
	if method == "" {
		return fmt.Errorf("must pass non-empty method arge")
	}
	if req == nil {
		return fmt.Errorf("must pass non-nil req arg")
	}
	if resp == nil {
		return fmt.Errorf("must pass non-nil resp arg")
	}
	if reflect.TypeOf(resp).Kind() != reflect.Ptr {
		return fmt.Errorf("resp must be a pointer")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if c.maxSize > 0 {
		if len(data) > int(c.maxSize) {
			return fmt.Errorf("data has a size greater than your max size limit")
		}
	}

	buff := c.pool.Get().(*bytes.Buffer)
	buff.Reset()
	defer c.pool.Put(buff)

	respBytes := buff.Bytes()
	if err := c.chunkRPC.Call(ctx, method, data, &respBytes); err != nil {
		return err
	}

	if err := json.Unmarshal(respBytes, resp); err != nil {
		return rpc.Errorf(rpc.ETBadData, "could not unmarshal into response(%T):\n%s", resp, string(respBytes))
	}
	return nil
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

// Server provides an json RPC server.
type Server struct {
	chunkRPC *rpc.Server
	maxSize  int64
	handlers map[string]RequestHandler
}

// NewServer is the constructor for a Server. rwc must be a *uds.Client or *uds.Conn.
func NewServer(socketAddr string, uid, gid int, fileMode os.FileMode) (*Server, error) {
	serv, err := rpc.NewServer(socketAddr, uid, gid, fileMode)
	if err != nil {
		return nil, err
	}

	return &Server{
		chunkRPC: serv,
		handlers: map[string]RequestHandler{},
	}, nil
}

// Start starts the server. This will block until the server stops.
func (s *Server) Start() error {
	return s.chunkRPC.Start()
}

// RegisterMethod registers an RPC method with the server.
func (s *Server) RegisterMethod(method string, handler RequestHandler) {
	s.chunkRPC.RegisterMethod(method, handler)
}
