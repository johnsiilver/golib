/*
Package rpc provides an RPC service for non-specific []byte in and []byte out data.
Note that this package is usually used to build higher-level rpc packages and not directly.

A simple client works like:
	client, err := New(pathToSocket)
	if err != nil {
		// Do something
	}

	ctx, cancel := context.WitheTimeout(5 * time.Second)
	resp := Response{}

	retry:
		// req and resp are []byte{}
		if err := client.Call(ctx, "method", req, &resp); err != nil {
			if Retryable(err) {
				// Okay, so you probably should do this in a for loop, but I wanted to use
				// a label for the hell of it.
				goto retry
			}
			// Do something here, cause you have a non-retryable error.
		}

Note: The server only returns errors to clients when something goes wrong. This makes it
predictable on the server side when the error is retryable. When the service
has an error, I recommend returning the expected response, which should have a dict containing
the error code and the error message. This allows your clients to decide if they should retry
a request.
*/
package rpc

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/chunk"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/chunk/rpc/payload"

	"github.com/panjf2000/ants/v2"
	"google.golang.org/protobuf/proto"
)

func payloadReset(p *payload.Payload) {
	data := p.Data
	p.Reset()
	p.Data = data
}

// Client provides an RPC client with []byte in and []byte out.
type Client struct {
	rwc     io.ReadWriteCloser
	chunker *chunk.Client
	pool    *chunk.Pool

	maxSize int64

	id uint32 // protected with atomic

	payloadPool *sync.Pool
	chPool      *sync.Pool

	mu      sync.Mutex
	pending map[uint32]chan *payload.Payload
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

// New is the constructor for Client. rwc must be a *uds.Client or *uds.Conn.
func New(socketAddr string, uid, gid int, fileModes []os.FileMode, options ...Option) (*Client, error) {
	rwc, err := uds.NewClient(socketAddr, uid, gid, fileModes)
	if err != nil {
		return nil, fmt.Errorf("client had problem opening uds socket: %w", err)
	}

	var client = &Client{
		rwc:     rwc,
		pending: map[uint32]chan *payload.Payload{},
		payloadPool: &sync.Pool{
			New: func() interface{} {
				return &payload.Payload{}
			},
		},
		chPool: &sync.Pool{
			New: func() interface{} {
				return make(chan *payload.Payload, 1)
			},
		},
	}

	for _, o := range options {
		o(client)
	}

	if client.pool == nil {
		client.pool = chunk.NewPool(10)
	}

	chunker, err := chunk.New(rwc, chunk.SharedPool(client.pool), chunk.MaxSize(client.maxSize))
	if err != nil {
		return nil, err
	}
	client.chunker = chunker

	go client.readAndRoute()

	return client, nil
}

// Close closes the underyling connection.
func (c *Client) Close() error {
	return c.rwc.Close()
}

// Call calls the RPC service. If Context timeout is not set, will default to 5 minutes.
func (c *Client) Call(ctx context.Context, method string, req []byte, resp *[]byte) error {
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

	if c.maxSize > 0 {
		if len(req) > int(c.maxSize) {
			return fmt.Errorf("data has a size greater than your max size limit")
		}
	}

	var cancel context.CancelFunc
	d, ok := ctx.Deadline()
	if !ok {
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		d, _ = ctx.Deadline()
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	id := atomic.AddUint32(&c.id, 1)

	p := c.payloadPool.Get().(*payload.Payload)
	p.Id = id
	p.ExpUnixNano = d.UnixNano()
	p.Method = method
	p.Data = req

	b, err := c.marshalProto(p)
	defer c.pool.Put(b)
	if err != nil {
		return fmt.Errorf("could not marshal the JSON payload for the request: %w", err)
	}
	c.payloadPool.Put(p)

	payCh := c.chPool.Get().(chan *payload.Payload)
	c.mu.Lock()
	c.pending[id] = payCh
	c.mu.Unlock()

	if err := c.chunker.Write(*b); err != nil {
		c.rwc.Close()
		return fmt.Errorf("chunk could not be written: %s", err)
	}

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	select {
	case <-ctx.Done():
		return Errorf(ETDeadlineExceeded, ctx.Err().Error())
	case p = <-payCh:
		if p.Id == 0 {
			return Errorf(ETBadData, "payload sent by the server could not be unmarshalled")
		}
		if p.ErrType != 0 {
			return Errorf(ErrType(p.ErrType), p.Err)
		}
		*resp = p.Data
		p.Reset()
		c.payloadPool.Put(p)
		return nil
	}
}

func (c *Client) readAndRoute() {
	for {
		buff, err := c.chunker.Read()
		if err != nil {
			if err != io.EOF {
				c.rwc.Close()
				log.Printf("chunk could not be read (client is now closed): %s", err)
			}
			return
		}

		p := c.payloadPool.Get().(*payload.Payload)
		if err := proto.Unmarshal(*buff, p); err != nil {
			log.Printf("received a set of bytes that could not be unmarshalled to a payload:\n%s", string(*buff))
			continue
		}
		c.chunker.Recycle(buff)

		c.mu.Lock()
		ch := c.pending[p.Id]
		c.mu.Unlock()
		// This happends if the call has already met a deadline, so we just drop this.
		if ch == nil {
			continue
		}
		ch <- p
	}
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

type credKeyType string

const credKey credKeyType = "credKey"

// CredFromCtx will extract the Cred from the Context object.
func CredFromCtx(ctx context.Context) uds.Cred {
	return ctx.Value(credKey).(uds.Cred)
}

// RequestHandler will receive a Context object with a Deadline set and you can retrieve
// the calling process creds with CredFromCtx.  An error returned is a ErrServer
// that can not be retried. Generally service errors should be in the resp, not as
// a returned error. See the note in the package intro.
type RequestHandler func(ctx context.Context, req []byte) (resp []byte, err error)

// Server provides an json RPC server.
type Server struct {
	socketAddr  string
	uid, gid    int
	fileMode    os.FileMode
	udsServ     *uds.Server
	pool        *chunk.Pool
	payloadPool *sync.Pool // *pb.Payload
	maxSize     int64
	handlers    map[string]RequestHandler
	stop        chan struct{}
	inFlight    sync.WaitGroup

	started bool
}

// NewServer is the constructor for a Server. rwc must be a *uds.Client or *uds.Conn.
func NewServer(socketAddr string, uid, gid int, fileMode os.FileMode) (*Server, error) {
	return &Server{
		socketAddr: socketAddr,
		uid:        uid,
		gid:        gid,
		fileMode:   fileMode,
		pool:       chunk.NewPool(10),
		payloadPool: &sync.Pool{
			New: func() interface{} {
				return &payload.Payload{}
			},
		},
		stop:     make(chan struct{}),
		handlers: map[string]RequestHandler{},
	}, nil
}

// Start starts the server. This will block until the server stops.
func (s *Server) Start() error {
	udsServ, err := uds.NewServer(s.socketAddr, s.uid, s.gid, s.fileMode)
	if err != nil {
		return err
	}
	s.udsServ = udsServ
	s.started = true

	for {
		select {
		case <-s.stop:
			return s.udsServ.Close()
		case conn := <-s.udsServ.Conn():
			ants.Submit(
				func() {
					s.handleRequests(conn)
				},
			)
		}
	}
}

// Stop stops the server, which will stop listening for new connections. This
// should slowly kill off existing calls. Stop will return when all calls have
// completed or the context deadline is reached(or cancelled). A nil error
// indicates that all jobs were completed. Note: a Server cannot be reused.
func (s *Server) Stop(ctx context.Context) error {
	close(s.stop)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.waitInFlight():
		return nil
	}
}

func (s *Server) waitInFlight() chan struct{} {
	ch := make(chan struct{})
	go func() {
		s.inFlight.Wait()
		close(ch)
	}()
	return ch
}

// RegisterMethod registers an RPC method with the server.
func (s *Server) RegisterMethod(method string, handler RequestHandler) {
	if _, ok := s.handlers[method]; ok {
		panic(fmt.Sprintf("cannot register method %s twice", method))
	}
	s.handlers[method] = handler
}

func (s *Server) handleRequests(conn *uds.Conn) {
	defer conn.Close()
	chunker, err := chunk.New(conn, chunk.SharedPool(s.pool), chunk.MaxSize(s.maxSize))
	if err != nil {
		log.Println(err)
		return
	}

	for {
		buff, err := chunker.Read()
		if err != nil {
			if err != io.EOF {
				// If the server has been closed, then this will be a *net.Op error.
				// We shouldn't error if the server was stopped.
				select {
				case <-s.stop:
					return
				default:
				}
				log.Println(err)
			}
			return
		}
		s.inFlight.Add(1)
		ants.Submit(
			func() {
				s.callHandler(buff, conn, chunker)
			},
		)
	}
}

// callHandler unloads our frame (payload type), looks up the method to call,
func (s *Server) callHandler(req *[]byte, conn *uds.Conn, chunker *chunk.Client) {
	defer s.inFlight.Done()
	defer chunker.Recycle(req)

	p := s.payloadPool.Get().(*payload.Payload)

	if err := proto.Unmarshal(*req, p); err != nil {
		servErrorf(chunker, p.Id, ETBadData, fmt.Errorf("got payload that could not be unmarshalled: %s", string(*req)))
		return
	}

	h, ok := s.handlers[p.Method]
	if !ok {
		servErrorf(chunker, p.Id, ETMethodNotFound, fmt.Errorf("method(%s): not found", p.Method))
		return
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, p.ExpUnixNano))
	ctx = context.WithValue(ctx, credKey, conn.Cred)
	defer cancel()

	resp, err := h(ctx, p.Data)
	if err != nil {
		servErrorf(chunker, p.Id, ETMethodNotFound, err)
		return
	}

	p.Data = resp // Just reuse the send payload, just replace the data.
	buff, err := s.marshalProto(p)
	p.Reset()
	s.payloadPool.Put(p)
	if err != nil {
		servErrorf(chunker, p.Id, ETServer, fmt.Errorf("could not marshal they payload: %s", err))
		return
	}
	defer s.pool.Put(buff)

	if err := chunker.Write(*buff); err != nil {
		return
	}
}

func (s *Server) marshalProto(m proto.Message) (*[]byte, error) {
	buff := s.pool.Get()

	b, err := marshalOpts.MarshalAppend(*buff, m)
	if err != nil {
		return nil, err
	}

	*buff = b
	return buff, nil
}

func servErrorf(chunker *chunk.Client, id uint32, code ErrType, err error) error {
	p := &payload.Payload{ErrType: uint32(code), Err: err.Error()}
	b, err := proto.Marshal(p) // Can't error
	if err != nil {
		panic("major bug")
	}

	return chunker.Write(b)
}
