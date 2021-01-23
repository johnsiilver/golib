/*
Package uds provides a server and client for Unix Domain Sockets. This provides a lot of convenience
around the "net" package for handling all the file setup and detecting closed connections. It also
provides the ability to authenticate connections.

The package currently only works for Linux/Darwin, as those are the systems I use.

This package takes the stance that Read() and Write() calls by default should infinitely block
unless the socket is closed. This eases development.

We also handle write only connections where Write() calls may not detect a closed connection. This can
be done setting the WriteOnly option.

This package is fairly straight forward in that you can uses server.Conn objects and Client objects
as io.ReadWriteClose objects. We also provide higher level clients that wrap this for convenience.

OSX Note:
	NewServer() is unable to chown the socket if the containing directory doesn't have
	a 0770 mask (operation not permitted). So opening in os.Tempdir() will fail.  You can simply
	put a sub-directory with those perms and it will work.

Unix/Linux Note:
	Socket paths may have a length limit that is different than the normal
	filesystem. On OSX, you can receive "bind: invalid argument" when the name is too long.

	On Linux there seems to be an 108 character for path names. https://github.com/golang/go/issues/6895 .
	I have set this as the default limit for all clients so I don't have to figure out the limit on
	every type of system and interpret non-sensical errors (invalid argument doesn't mean all that much).
*/
package uds

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/kylelemons/godebug/pretty"
)

// oneByte is used as the receiver for a read that will never succeed. Because the read must be at
// least a single byte, this exists to prevent any heap allocations.
var oneByte = make([]byte, 1)

// ID represents a numeric ID. Go in various libraries stores IDs such as Uid or Gid as strings.
// However in other more OS specific libraries, it might be int or int32. This simply unifies that
// so it is easier to translate for whatever need you have. The string representation is probably
// a compatibility feature to allow an OS that doesn't use numeric IDs to work. However I am not
// interested in supporting those in this package.
type ID int

// String returns the ID as a string.
func (i ID) String() string {
	return strconv.Itoa(int(i))
}

// Int returns the ID as an int.
func (i ID) Int() int {
	return int(i)
}

// Int32 returns the ID as an int32.
func (i ID) Int32() int32 {
	return int32(i)
}

// Current provides information about the current process and user.
func Current() (Cred, *user.User, error) {
	u, err := user.Current()
	if err != nil {
		return Cred{}, nil, err
	}

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	cred := Cred{
		PID: ID(os.Getpid()),
		UID: ID(uid),
		GID: ID(gid),
	}
	return cred, u, nil
}

// Cred provides the credentials of the local process contacting the server.
type Cred struct {
	// PID is the process id of the process.
	PID ID
	// UID is the user id of the process.
	UID ID
	// GID is the group id of the process.
	GID ID
}

// Conn represents a UDS connection from a client. Must take a pointer if this will be copied
// after being received.
type Conn struct {
	Cred          Cred
	conn          *net.UnixConn
	readDeadline  time.Time
	writeDeadline time.Time
	writeOnly     bool
}

// WriteOnly let's the Conn know that this Conn will only be used for writing. This will cause a
// *special* read to happen on all writes to make sure the connection isn't closed, as writes are
// not guaranteed to error on a UDS. If doing reads, this isn't required. If this is set and
// you read from the Conn, Read() will panic.
func (c *Conn) WriteOnly() {
	c.writeOnly = true
}

// UnixConn will return the underlying UnixConn object. You can use this to use its underlying
// methods or change buffering. Note that .SetReadDeadline()/.SetWriteDeadline()/.SetDeadline()
// will not do anything. Use ReadTimeout()/ReadDeadline()/WriteTimeout()/WriteDeadline()
// defined on Conn.
func (c *Conn) UnixConn() *net.UnixConn {
	return c.conn
}

// Read implements io.Reader.Read(). This has an inifite read timeout. If you want to have a timeout,
// call ReadDeadline() or ReadTimeout() before calling. You must do this for every Read() call.
func (c *Conn) Read(b []byte) (int, error) {
	if c.writeOnly {
		panic("called Read() when Client.WriteOnly() set")
	}
	c.conn.SetReadDeadline(c.readDeadline)
	c.readDeadline = time.Time{}

	return c.conn.Read(b)
}

// ReadByte implements io.ByteReader.
func (c *Conn) ReadByte() (byte, error) {
	if c.writeOnly {
		panic("called Read() when Client.WriteOnly() set")
	}
	c.conn.SetReadDeadline(c.readDeadline)
	c.readDeadline = time.Time{}
	b := make([]byte, 1)
	_, err := c.conn.Read(b)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// ReadTimeout caused the next Read() call to timeout at time.Now().Add(timeout). Must be used
// before every Read() call that you want to have a timeout.
func (c *Conn) ReadTimeout(timeout time.Duration) {
	c.readDeadline = time.Now().Add(timeout)
}

// ReadDeadline caused the next Read() call to timeout at t. Must be used
// before every Read() call that you want to have a timeout.
func (c *Conn) ReadDeadline(t time.Time) {
	c.readDeadline = t
}

// WriteTimeout caused the next Write() call to timeout at time.Now().Add(timeout). Must be used
// before every Write() call that you want to have a timeout.
func (c *Conn) WriteTimeout(timeout time.Duration) {
	c.writeDeadline = time.Now().Add(timeout)
}

// WriteDeadline caused the next Write() call to timeout at t. Must be used
// before every Write() call that you want to have a timeout.
func (c *Conn) WriteDeadline(t time.Time) {
	c.writeDeadline = t
}

// Write implements io.Writer.Write(). This has an inifite write timeout. If you want to have a timeout,
// call WriteDeadline() or WriteTimeout() before calling. You must do this for every Write() call.
func (c *Conn) Write(b []byte) (int, error) {
	if c.writeOnly {
		if isClosed(c.conn) {
			return 0, io.EOF
		}
	}
	c.conn.SetWriteDeadline(c.writeDeadline)
	c.writeDeadline = time.Time{}

	return c.conn.Write(b)
}

// Close implements io.Closer.Close().
func (c Conn) Close() error {
	return c.conn.Close()
}

// Server provides a Unix Domain Socket server that clients can connect on.
type Server struct {
	l       *net.UnixListener
	oneByte []byte
	errCh   chan error
	connCh  chan Conn
}

// NewServer creates a new UDS server that creates and listens to the file at socketPath. uid and gid are
// the uid and gid that file will be set to and fileMode is the file mode it will inherit. If
// sockerAddr exists this will attempt to delete it. Suggest fileMode of 0770.
func NewServer(socketAddr string, uid, gid int, fileMode os.FileMode) (*Server, error) {
	if len([]rune(socketAddr)) >= 108 {
		return nil, fmt.Errorf("socketAddr(%s) path length must be 108 characters or less", socketAddr)
	}

	if err := os.Remove(socketAddr); err != nil {
		if _, ok := err.(*os.PathError); !ok {
			// if !errors.Is(err, &os.PathError{}) { // I don't know why this doesn't work.
			return nil, fmt.Errorf("unable to create server socket(%s), could not remove old socket file: %s", socketAddr, err)
		}
	}

	// This is sketchy, but it is how its done:
	// https://github.com/golang/go/issues/11822
	syscall.Umask(0770)

	l, err := net.Listen("unix", socketAddr)
	if err != nil {
		return nil, fmt.Errorf("unable to create server socket(%s): %w", socketAddr, err)
	}

	stat, err := os.Stat(socketAddr)
	if err != nil {
		panic(err)
	}
	log.Println(stat.Mode())
	log.Println(pretty.Sprint(stat.Sys()))
	log.Println(uid)
	log.Println(gid)

	if err := os.Chmod(socketAddr, fileMode); err != nil {
		l.Close()
		return nil, fmt.Errorf("unable to create server socket(%s), could not chmod the socket file: %s", socketAddr, err)
	}
	stat, err = os.Stat(socketAddr)
	if err != nil {
		panic(err)
	}
	log.Println(stat.Mode())

	/*
		if err := os.Chown(socketAddr, uid, gid); err != nil {
			l.Close()
			return nil, fmt.Errorf("unable to create server socket(%s), could not chown the socket file: %s", socketAddr, err)
		}
	*/

	serv := &Server{
		l:       l.(*net.UnixListener),
		oneByte: make([]byte, 1),
		errCh:   make(chan error, 1),
		connCh:  make(chan Conn, 1),
	}
	go serv.accept()
	return serv, nil
}

// Conn returns a channel that is populated with connection to the server. The channel is closed
// when the server's is no longer serving.
func (c *Server) Conn() chan Conn {
	return c.connCh
}

// Close stops listening for connections on the socket.
func (c *Server) Close() error {
	return c.l.Close()
}

// Closed returns a channel that returns an error when the connection to the server is closed.
// This can be because you have called Close(), the socket had a read error or the socket file was
// removed. An io.EOF error will not be returned (as this is normal operation). Normally this is
// used to block and return the final status of the server.
func (c *Server) Closed() chan error {
	return c.errCh
}

func (c *Server) accept() {
	go func() {
		defer close(c.connCh)
		defer close(c.errCh)
		for {
			conn, err := c.l.Accept()
			if err != nil {
				c.l.Close()
				if err != io.EOF {
					// This seems to be the error that happens once a conn is closed for a UDS listener.
					var opErr *net.OpError
					if errors.As(err, &opErr) && opErr.Op != "accept" {
						c.errCh <- err
					}
				}
				return
			}
			uc := conn.(*net.UnixConn)
			cred, err := readCreds(uc)
			if err != nil {
				log.Println("unable to read creds from socket client, rejecting conn")
				conn.Close()
				continue
			}
			c.connCh <- Conn{conn: uc, Cred: cred}
		}
	}()
}

// Client provides a UDS client for connecting to a UDS server.
type Client struct {
	conn                        *net.UnixConn
	readDeadline, writeDeadline time.Time
	writeOnly                   bool
}

// NewClient creates a new UDS client to the socket at socketAddr that must have the uid and gid specified.
// fileModes provides a list of acceptable file modes that the socket can be in (suggest 0770, 1770).
func NewClient(socketAddr string, uid, gid int, fileModes []os.FileMode) (*Client, error) {
	stats, err := os.Stat(socketAddr)
	if err != nil {
		return nil, fmt.Errorf("could not stat socket address(%s): %w", socketAddr, err)
	}

	switch stats.Mode() {
	case 0770, 1770:
		return nil, fmt.Errorf("socket address(%s) had incorrect mode(%v), must be 0770", socketAddr, stats.Mode())
	}

	conn, err := net.Dial("unix", socketAddr)
	if err != nil {
		return nil, fmt.Errorf("unable to dial socket(%s): %w", socketAddr, err)
	}
	uc := conn.(*net.UnixConn)

	return &Client{conn: uc}, nil
}

// WriteOnly let's the Client know that it will only be used for writing. This will cause a
// *special* read to happen on all writes to make sure the connection isn't closed, as writes are
// not guaranteed to error on a UDS. If doing reads, this isn't required. If this is set and
// you read fro the Conn the read will panic.
func (c *Client) WriteOnly() {
	c.writeOnly = true
}

// UnixConn will return the underlying UnixConn object. You can use this to use its underlying
// methods or change buffering. Note that SetReadDeadline/SetWriteDeadline/SetDeadline do not work,
// use one of Client's methods instead.
func (c *Client) UnixConn() *net.UnixConn {
	return c.conn
}

// Read implements io.Reader.Read(). This will block until it has read into the buffer. This
// differs from native behavior which times out. If you want to have read timeouts, use
// ReadDeadline()/ReadTimeout() to change. This must be done before every Read() call.
func (c *Client) Read(b []byte) (int, error) {
	if c.writeOnly {
		panic("called Read() when Client.WriteOnly() set")
	}
	c.conn.SetReadDeadline(c.readDeadline)
	c.readDeadline = time.Time{}
	return c.conn.Read(b)
}

// ReadByte implements io.ByteReader.
func (c *Client) ReadByte() (byte, error) {
	if c.writeOnly {
		panic("called Read() when Client.WriteOnly() set")
	}
	c.conn.SetReadDeadline(c.readDeadline)
	c.readDeadline = time.Time{}
	b := make([]byte, 1)
	_, err := c.conn.Read(b)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// ReadTimeout caused the next Read() call to timeout at time.Now().Add(timeout). Must be used
// before every Read() call that you want to have a timeout.
func (c *Client) ReadTimeout(timeout time.Duration) {
	c.readDeadline = time.Now().Add(timeout)
}

// ReadDeadline caused the next Read() call to timeout at t. Must be used
// before every Read() call that you want to have a timeout.
func (c *Client) ReadDeadline(t time.Time) {
	c.readDeadline = t
}

// WriteTimeout caused the next Write() call to timeout at time.Now().Add(timeout). Must be used
// before every Write() call that you want to have a timeout.
func (c *Client) WriteTimeout(timeout time.Duration) {
	c.writeDeadline = time.Now().Add(timeout)
}

// WriteDeadline caused the next Write() call to timeout at t. Must be used
// before every Write() call that you want to have a timeout.
func (c *Client) WriteDeadline(t time.Time) {
	c.writeDeadline = t
}

// Write implements io.Reader.Writer(). This will block until it has written the buffer. This
// differs from native behavior which times out. If you want to have write timeouts, use
// WriteDeadeline()/WriteTimeout() to change. This must be done before every Write() call.
func (c *Client) Write(b []byte) (int, error) {
	if c.writeOnly {
		if isClosed(c.conn) {
			return 0, io.EOF
		}
	}
	c.conn.SetWriteDeadline(c.writeDeadline)
	c.writeDeadline = time.Time{}
	return c.conn.Write(b)
}

// Close closes the connection to the server.
func (c *Client) Close() error {
	return c.conn.Close()
}

// isClosed tests the connection to see if it is open by trying to read from it.
// We do this because this connection is actually one way and we normally don't read.
// This will never actually read anything, but you can't use a 0 byte slice because that will never error.
// Writes do not block on a broken conn.
func isClosed(conn *net.UnixConn) bool {
	conn.SetReadDeadline(time.Now())
	if _, err := conn.Read(oneByte); err == io.EOF {
		return true
	}
	return false
}
