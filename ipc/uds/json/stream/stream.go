/*
Package json provides a client that can be used on a *uds.Client or *uds.Conn to send streaming JSON values.
This package uses chunk underneath and therefore expects the other side to understand its data format.

This is for decoding a single message type that can be made up of sub-messages. If you wish to
decode JSON dicts inside a dict stream or arbitrary JSON messages, simply wrap
*uds.Client or *uds.Conn with json.Decoder instead of using this package.

This handles streams beautifully if you define a simple master message containing sub-messages. Aka:
	type Message struct {
		Type int8
		SubMessage1 SubMessage1
		SubMessage2 SubMessage2
	}
	...

	Where Type will indicate what message field is set.  Streaming this via client.Write() calls allows
	adding new types of messages any time and receivers detecting types a client may not support.
	0 should indicate that the Message is the zero value.
*/
package stream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/chunk"
)

// Client provides a wrapper around an *uds.Client or *uds.Server that can send and receive JSON messages.
type Client struct {
	rwc     io.ReadWriteCloser
	chunker *chunk.Client
	pool    *sync.Pool

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
		client.pool = &sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		}
	}

	chunker, err := chunk.New(rwc, chunk.SharedPool(client.pool), chunk.MaxSize(client.maxSize))
	if err != nil {
		return nil, err
	}
	client.chunker = chunker

	return client, nil
}

// Read reads the next JSON message into value v.
func (c *Client) Read(v interface{}) error {
	buff, err := c.chunker.Read()
	if err != nil {
		c.rwc.Close()
		return err
	}

	defer c.chunker.Recycle(buff)
	return json.Unmarshal(buff.Bytes(), v)
}

// Write writes v as a JSON value into the socket.
func (c *Client) Write(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	return c.chunker.Write(b)
}
