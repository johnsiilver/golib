/*
Package stream provides a client that can be used on a *uds.Client or *uds.Conn to send streaming binary
proto values. This package uses chunk underneath and therefore expects the other side to understand
its data format.
*/
package stream

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/chunk"
)

type msgType uint8

const (
	mtUnknown  msgType = 0
	mtData     msgType = 1
	mtClose    msgType = 2
	mtCloseAck msgType = 3
)

type streamMsg[T any] struct {
	msg T
	err error
}

type controlMsg struct {
	msgType msgType
}

// Client provides a wrapper around an *uds.Client or *uds.Server that can send and receive PROTO messages.
type Client struct {
	ctx    context.Context
	cancel context.CancelFunc

	rwc     io.ReadWriteCloser
	chunker *chunk.Client
	pool    *chunk.Pool

	// sentClose is set when we send a message telling the other side we are closing our send connection.
	sentClose atomic.Bool
	// recvAck is set when we receive a message telling us the other side has acknowledged our close.
	recvAck atomic.Bool

	readCh         chan streamMsg[*[]byte]
	controlCh      chan controlMsg
	goroutineCount sync.WaitGroup
	sigCloseReadCh chan struct{}
	sendRecvErr    error
	errMu          sync.Mutex

	sentClosed chan struct{}
	errSeen    chan struct{}
	// recvClose is set when we receive a message telling us the other side is closing their send connection.
	recvClose chan struct{}

	maxSize int64

	clientTestName string
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
// This is useful when clients are short lived and have similar message sizes.
func SharedPool(pool *chunk.Pool) Option {
	return func(c *Client) {
		c.pool = pool
	}
}

func setTestName(name string) Option {
	return func(c *Client) {
		c.clientTestName = name
	}
}

// New is the constructor for Client. rwc must be a *uds.Client or *uds.Conn.
func New(rwc io.ReadWriteCloser, options ...Option) (client *Client, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if client == nil {
			cancel()
		}
	}()

	switch rwc.(type) {
	case *uds.Client, *uds.Conn:
	default:
		return nil, fmt.Errorf("rwc was not a *uds.Client or *uds.Server, was %T", rwc)
	}

	client = &Client{
		ctx:            ctx,
		cancel:         cancel,
		rwc:            rwc,
		sentClosed:     make(chan struct{}),
		recvClose:      make(chan struct{}),
		sigCloseReadCh: make(chan struct{}),
		readCh:         make(chan streamMsg[*[]byte], 1),
		controlCh:      make(chan controlMsg, 1),
		errSeen:        make(chan struct{}),
	}
	for _, o := range options {
		o(client)
	}

	if client.pool == nil {
		client.pool = chunk.NewPool(10)
	}

	// This code uses the pool from the client, so we need to create the chunker after the client and
	// then set the client's chunker.
	chunker, err := chunk.New(rwc, chunk.SharedPool(client.pool), chunk.MaxSize(client.maxSize))
	if err != nil {
		return nil, err
	}
	client.chunker = chunker

	// Spins off our readers to read from the connection and handle control messages.
	go client.controlHandle()
	go client.reader()

	// When all reading goroutines are done and we have send our close, we close the chunker.
	client.goroutineCount.Add(2)
	go func() {
		client.goroutineCount.Wait()
		<-client.sentClosed
		client.chunker.Close()
		// log.Printf("%s: goroutineCount.Wait() done", client.clientTestName)
	}()

	return client, nil
}

// CloseSend closes the send side of the connection. The Context can be used to timeout waiting for the
// other side to acknowledge the close. This is not thread safe and you cannot call it multiple times.
func (c *Client) CloseSend(ctx context.Context) error {
	// log.Printf("%s: CloseSend() called", c.clientTestName)
	if c.err() != nil {
		return c.err()
	}

	if !c.sentClose.Load() {
		c.sentClose.Store(true)

		if err := c.chunker.Write(*closeMsg); err != nil {
			c.setErr(err)
			return err
		}
	}
	// log.Printf("%s: waiting on sendIsClosed", c.clientTestName)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.errSeen:
		return c.err()
	case <-c.sentClosed:
	}
	// log.Printf("%s: sendIsClosed closed", c.clientTestName)
	return nil
}

// Read reads the next proto message into message m. This is not thread safe, though you can call Read() and Write()
// concurrently.
func (c *Client) Read(ctx context.Context, m proto.Message) error {
	if c.err() != nil {
		// log.Println("Read(): err() != ", c.err())
		return c.err()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg, ok := <-c.readCh:
		if !ok {
			if err := c.err(); err != nil {
				// log.Println("Reach channel closed and c.err(): ", err)
				return err
			}
			return io.EOF
		}
		if msg.err != nil {
			return msg.err
		}
		defer c.chunker.Recycle(msg.msg)
		// Note: buff's first byte is the message type. Must skip it when unmarshaling.
		if err := proto.Unmarshal((*msg.msg)[1:], m); err != nil {
			return fmt.Errorf("could not unmarshal proto: %s", err)
		}
	}
	return nil
}

// Write writes m as a binary proto message into the socket. This is not thread safe, though you can call Read() and Write()
// concurrently.
func (c *Client) Write(ctx context.Context, m proto.Message) error {
	if err := c.err(); err != nil {
		return err
	}

	if c.sentClose.Load() {
		return fmt.Errorf("cannot write, already closed")
	}

	b, err := c.marshalFrame(mtData, m)
	if err != nil {
		return err
	}
	defer c.pool.Put(b)

	if err := c.chunker.Write(*b); err != nil {
		c.setErr(err)
		// log.Printf("%s: chunkerWrite error:", err)
		return err
	}
	return nil
}

// closeMsg is the message we send to the other side to tell them we are closing our send connection.
var closeMsg = &[]byte{byte(mtClose)}

// closeAckMsg is the message we send to the other side to acknowledge their close.
var closeAckMsg = &[]byte{byte(mtCloseAck)}

// reader reads messages from the connection and sends them to the readCh or controlCh channel.
func (c *Client) reader() {
	// defer log.Printf("%s: reader goroutine done", c.clientTestName)
	defer c.goroutineCount.Done()
	defer close(c.readCh)
	defer close(c.controlCh)

	type readAnswer struct {
		msg     *[]byte
		msgType msgType
	}

	// chunkRead reads the next chunk from the connection and returns it on a channel.
	// This allows us to receive a channel close method and then return.
	chunkRead := func() chan streamMsg[readAnswer] {
		ch := make(chan streamMsg[readAnswer], 1)
		go func() {
			defer close(ch)
			t, buff, err := c.read(c.ctx)
			if err != nil {
				if io.EOF == err {
					return
				}
				select {
				case <-c.errSeen:
					return
				case <-c.ctx.Done():
					return
				case ch <- streamMsg[readAnswer]{err: err}:
					return
				}
			}
			ch <- streamMsg[readAnswer]{msg: readAnswer{msg: buff, msgType: t}}
		}()
		return ch
	}

	for {
		select {
		case <-c.errSeen:
			return
		case <-c.ctx.Done():
			return
		case <-c.sigCloseReadCh:
			return
		case sm, ok := <-chunkRead():
			if !ok {
				return
			}
			if sm.err != nil {
				if strings.HasSuffix(sm.err.Error(), "use of closed network connection") {
					return
				}
				c.setErr(sm.err)
				// log.Printf("%s: reader exit error: %s", c.clientTestName, sm.err)
				return
			}
			switch sm.msg.msgType {
			case mtData:
				c.readCh <- streamMsg[*[]byte]{msg: sm.msg.msg}
			default:
				c.controlCh <- controlMsg{msgType: sm.msg.msgType}
			}
		}
	}
}

// controlHandle handles control messages from the other side.
func (c *Client) controlHandle() {
	seenMTClose := false
	seenMTCloseAck := false

	defer log.Printf("%s: controlHandle() exiting", c.clientTestName)
	defer c.goroutineCount.Done()

	for {
		// If we've sent a close and received an ack and they've sent a close and we sent an ack, we are done.
		if seenMTClose && seenMTCloseAck {
			return
		}
		select {
		case <-c.errSeen:
			return
		case mt, ok := <-c.controlCh:
			if !ok {
				return
			}
			switch mt.msgType {
			// The other side is closing their send connection.
			case mtClose:
				// log.Printf("%s: received mtClose", c.clientTestName)
				if seenMTClose {
					c.setErr(fmt.Errorf("received mtClose twice"))
					return
				}
				seenMTClose = true

				// Send an ack.
				if err := c.chunker.Write(*closeAckMsg); err != nil {
					c.setErr(err)
					return
				}
				close(c.sigCloseReadCh)
			// The other side is acknowledging our SendClose.
			case mtCloseAck:
				if seenMTCloseAck {
					c.setErr(fmt.Errorf("received mtCloseAck twice"))
					return
				}
				seenMTCloseAck = true

				// log.Printf("%s: received mtCloseAck", c.clientTestName)
				c.recvAck.Store(true)
				close(c.sentClosed)
			default:
				err := fmt.Errorf("received unknown message type: %d", mt.msgType)
				c.setErr(err)
				// log.Printf("%s: reader exit error: %s", c.clientTestName, err)
				return
			}
		}
	}
}

// read reads the next message from the connection. It returns the message type, the message and an error.
func (c *Client) read(ctx context.Context) (msgType, *[]byte, error) {
	buff, err := c.chunker.Read()
	if err != nil {
		return mtUnknown, nil, err
	}

	// If we received a chunk of no size, we close the connection. We should never receive a chunk of no size, as
	// we always get a byte for the message type.
	if len(*buff) == 0 {
		return mtUnknown, nil, fmt.Errorf("received a chunk of no size")
	}

	dt := msgType((*buff)[0])

	switch dt {
	case mtData:
		return mtData, buff, nil
	// The other side is closing their send connection.
	case mtClose:
		return mtClose, nil, nil
	// The other side is acknowledging our SendClose.
	case mtCloseAck:
		return mtCloseAck, nil, nil
	}
	return mtUnknown, nil, fmt.Errorf("received unknown message type: %d", dt)
}

func (c *Client) setErr(err error) {
	c.errMu.Lock()
	defer c.errMu.Unlock()

	// If we have sent a close, received our Ack and received their close, we ignore the error.
	if strings.HasSuffix(err.Error(), "use of closed network connection") {
		if c.sentClose.Load() && c.recvAck.Load() {
			select {
			case <-c.recvClose:
				return
			default:
			}
		}
	}

	if c.sendRecvErr != nil {
		return
	}
	c.sendRecvErr = err
	close(c.errSeen)
}

func (c *Client) err() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()

	return c.sendRecvErr
}

var marshalOpts = proto.MarshalOptions{}

func (c *Client) marshalFrame(msgType msgType, m proto.Message) (*[]byte, error) {
	buff := c.pool.Get()
	*buff = append(*buff, byte(msgType))

	b, err := marshalOpts.MarshalAppend(*buff, m)
	if err != nil {
		return nil, err
	}

	*buff = b
	return buff, nil
}
