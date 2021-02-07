/*
Package proto provides a client that can be used on a *uds.Client or *uds.Conn to send streaming binary
proto values. This package uses chunk underneath and therefore expects the other side to understand
its data format.

This is for decoding a single proto message type.
*/
package proto

import (
	"fmt"
	"io"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/chunk"

	"google.golang.org/protobuf/proto"
)

// Client provides a wrapper around an *uds.Client or *uds.Server that can send and receive JSON messages.
type Client struct {
	rwc     io.ReadWriteCloser
	chunker *chunk.Client
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

// New is the constructor for Client. rwc must be a *uds.Client or *uds.Conn.
func New(rwc io.ReadWriteCloser, options ...Option) (*Client, error) {
	var client *Client

	switch rwc.(type) {
	case *uds.Client, *uds.Conn:
		client = &Client{rwc: rwc}
	default:
		return nil, fmt.Errorf("rwc was not a *uds.Client or *uds.Server, was %T", rwc)
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

	return client, nil
}

// Read reads the next proto message into message m.
func (c *Client) Read(m proto.Message) error {
	buff, err := c.chunker.Read()
	if err != nil {
		c.rwc.Close()
		return err
	}

	defer c.chunker.Recycle(buff)
	return proto.Unmarshal(*buff, m)
}

// Write writes m as a binary proto message into the socket.
func (c *Client) Write(m proto.Message) error {
	b, err := c.marshalProto(m)
	if err != nil {
		return err
	}
	defer c.pool.Put(b)

	return c.chunker.Write(*b)
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
