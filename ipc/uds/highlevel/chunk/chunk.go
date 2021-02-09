/*
Package chunk provides a high level chunking client. A chunk is defined as a defined amount of data.
A chunk client will work with *uds.Client, *uds.Conn object for chunking in clients and servers.
It will work with other io.ReadWriteCloser, but we make not guarantees with those.

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
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/johnsiilver/golib/ipc/uds"
)

var ordering = binary.LittleEndian

type writer struct {
	buff *[]byte
}

func (w writer) Write(b []byte) (int, error) {
	*w.buff = append(*w.buff, b...)
	return len(b), nil
}

// Pool is a memory pool for buffers.
type Pool struct {
	ch   chan *[]byte
	pool *sync.Pool
}

// NewPool is the constructor for Pool. cap is capacity of an internal free list
// before using a sync.Pool.
func NewPool(cap int) *Pool {
	var ch chan *[]byte
	if cap > 0 {
		ch = make(chan *[]byte, cap)
	}
	p := &Pool{ch: ch}
	p.pool = &sync.Pool{New: p.alloc}
	return p
}

func (p *Pool) alloc() interface{} {
	return &[]byte{}
}

// Get gets some buffer out of the pool.
func (p *Pool) Get() *[]byte {
	select {
	case b := <-p.ch:
		return b
	default:
	}
	return p.pool.Get().(*[]byte)
}

// Put puts buffer back into the pool.
func (p *Pool) Put(b *[]byte) {
	*b = (*b)[:0]
	select {
	case p.ch <- b:
		return
	default:
		p.pool.Put(b)
	}
}

// Client provides a wrapper around an *uds.Client or *uds.Server that can send data chunks.
type Client struct {
	rwc  io.ReadWriteCloser
	pool *Pool

	writeVarInt []byte
	readVarInt  []byte

	maxSize int64

	closeOnce sync.Once
	closed    chan struct{}

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
// pool does not return a *[]byte object.
func SharedPool(pool *Pool) Option {
	return func(c *Client) {
		c.pool = pool
	}
}

// New is the constructor for Client. rwc must be a *uds.Client or *uds.Conn.
func New(rwc io.ReadWriteCloser, options ...Option) (*Client, error) {
	client := &Client{
		rwc:         rwc,
		writeVarInt: make([]byte, 8),
		closed:      make(chan struct{}),
	}

	for _, o := range options {
		o(client)
	}
	if client.pool == nil {
		client.pool = NewPool(10)
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

// ClosedSignal returns a channel that will be closed when this Client becomes closed.
func (c *Client) ClosedSignal() chan struct{} {
	return c.closed
}

// Close closes the underlying io.ReadWriteCloser.
func (c *Client) Close() error {
	defer c.closeOnce.Do(
		func() {
			close(c.closed)
		},
	)
	return c.rwc.Close()
}

// Recycle recycles a *[]byte. This should only be done when the Buffer is no longer used.
func (c *Client) Recycle(b *[]byte) {
	*b = (*b)[:0]
	c.pool.Put(b)
}

const (
	oneKiB = 1024
	oneMiB = 1000 * 1024
)

// Read reads the next message from the socket. Any error closes the client.
func (c *Client) Read() (*[]byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	size, err := binary.ReadVarint(c.rwc.(io.ByteReader))
	if err != nil {
		c.Close()
		return nil, err
	}

	if c.maxSize > 0 {
		if size > c.maxSize {
			c.Close()
			return nil, fmt.Errorf("message is larger than maximum size allowed")
		}
	}

	b := c.pool.Get()
	w := writer{b}

	i, err := io.CopyN(w, c.rwc, size)
	if err != nil {
		return nil, fmt.Errorf("could not read full chunk: %s", err)
	}
	*b = (*b)[:i]
	return b, nil
}

// Write writes b as a chunk into the socket. Any error closes the client.
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
		c.Close()
		return err
	}
	n, _ = c.rwc.Write(b)
	return err
}
