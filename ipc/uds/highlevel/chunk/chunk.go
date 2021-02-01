/*
Package chunk provides a high level chunking client. A chunk is defined as a defined amount of data.
A chunk client will work with *uds.Client, *uds.Conn object for chunking in clients and servers.

The high level RPC clients in all packages use chunk underneath.

A chunk client simply writes a variadic int to indicate the next message size to be written and then
writes the message. On a read call it reads the int indicating the size and then reads in that
amount of buffer.

The format is:
	[varint][data chunk]

The chunk client can set a maximum message size.

If a read error occurs, the chunk client will close the connection.  Read errors should only happen
if the maximum message size is exceeded or io.EOF to indicate the socket is closed.
*/
package chunk

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/johnsiilver/golib/ipc/uds"
)

var ordering = binary.LittleEndian

// Client provides a wrapper around an *uds.Client or *uds.Server that can send data chunks.
type Client struct {
	rwc  io.ReadWriteCloser
	pool *sync.Pool

	writeVarInt []byte
	readVarInt  []byte

	maxSize             int64
	biggestReadMsgSize  int
	biggestWriteMsgSize int

	readMu, writeMu sync.Mutex
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
		client = &Client{
			rwc:         rwc,
			writeVarInt: make([]byte, 8),
		}
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

	switch v := rwc.(type) {
	case *uds.Conn:
		v.UnixConn().SetReadBuffer(5 * 1024 * 1000)
		v.UnixConn().SetWriteBuffer(5 * 1024 * 1000)
	case *uds.Client:
		v.UnixConn().SetReadBuffer(5 * 1024 * 1000)
		v.UnixConn().SetWriteBuffer(5 * 1024 * 1000)
	}

	return client, nil
}

// Recycle recycles a *bytes.Buffer. This should only be done when the Buffer is no longer
// in use (including its internal []byte slice).
func (c *Client) Recycle(b *bytes.Buffer) {
	b.Reset()
	c.pool.Put(b)
}

const (
	oneKiB = 1024
	oneMiB = 1000 * 1024
)

// Read reads the next message from the socket.
func (c *Client) Read() (*bytes.Buffer, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	size, err := binary.ReadVarint(c.rwc.(io.ByteReader))
	if err != nil {
		c.rwc.Close()
		return nil, err
	}

	if c.maxSize > 0 {
		if size > c.maxSize {
			c.rwc.Close()
			return nil, fmt.Errorf("message is larger than maximum size allowed")
		}
	}

	buff := c.pool.Get().(*bytes.Buffer)
	buff.Reset()

	_, err = io.CopyN(buff, c.rwc, size)
	if err != nil {
		return nil, fmt.Errorf("could not read full chunk: %s", err)
	}

	return buff, nil
}

// Write writes b as a chunk into the socket.
func (c *Client) Write(b []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if len(b) == 0 {
		return nil
	}

	if c.maxSize > 0 {
		if len(b) > int(c.maxSize) {
			return fmt.Errorf("message exceeds max size set")
		}
	}

	n := binary.PutVarint(c.writeVarInt, int64(len(b)))
	_, err := c.rwc.Write(c.writeVarInt[:n])
	if err != nil {
		c.rwc.Close()
		return err
	}
	n, _ = c.rwc.Write(b)
	return err
}