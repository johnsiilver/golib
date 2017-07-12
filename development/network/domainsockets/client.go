package domainsockets

import (
	"time"

	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sync"

	log "github.com/golang/glog"
	"github.com/pborman/uuid"
	"golang.org/x/net/context"
)

type request struct {
	ctx  context.Context
	msg  *ClientMsg
	resp chan response
}

type response struct {
	msg *ServerMsg
	err error
}

// Client provides a client to a Domain Socket server.
type Client struct {
	inConn  *net.UnixConn
	outConn *bufio.Reader
	reqCh   chan request
	done    chan struct{}

	bufferSize int

	mu         sync.Mutex // mu protects everything below.
	responses  map[uint64]chan response
}

// DialServer dials a Unix Domain Socket where a server is listening and
// returns a client to the server.
func DialServer(uid string) (*Client, error) {
	// Have the client start listening for responses from the server.
	clientUID := uuid.New()
	clientPath := path.Join(os.TempDir(), clientUID)
	outConn, err := net.ListenUnixgram(udsType, &net.UnixAddr{clientPath, udsType})
	if err != nil {
		return nil, err
	}

	// Dial the server.
	log.Infof("client: dialing the server")
	setupConn, err := net.DialUnix(
		udsType,
		nil,
		&net.UnixAddr{path.Join(os.TempDir(), uid), udsType},
	)
	if err != nil {
		return nil, err
	}

	log.Infof("client: sending uid to server")
	if err := SetupEncode(clientUID, setupConn); err != nil {
		return nil, err
	}
	setupConn.Close()

	// Get the socket the server is going to listen on.
	out := bufio.NewReader(outConn)
	inUUID, err := SetupDecode(out)
	if err != nil {
		return nil, err
	}
	log.Infof("client: received server uid for conn")

	// Dial the server.
	in, err := net.DialUnix(
		udsType,
		nil,
		&net.UnixAddr{path.Join(os.TempDir(), inUUID), udsType},
	)
	if err != nil {
		return nil, err
	}
	log.Infof("client: dialed server")

	c := &Client{
		inConn:    in,
		outConn:   out,
		reqCh:     make(chan request, 50),
		responses: make(map[uint64]chan response),
	}

  if err := c.inConn.SetWriteBuffer(5 * MiB); err != nil {
    return nil, fmt.Errorf("cannot set the Unix Domain Socket buffer to 5 MiB")
  }

	go c.send()
	go c.receive()
	return c, nil
}

// Call makes a call to the remote procedure "call" with arguments in "msg".
// It returns the server's response or error.
// TODO(jdoak): Signal the far side of a context cancel.
func (c *Client) Call(ctx context.Context, call string, msg *ClientMsg) (*ServerMsg, error) {
	msg.Type = ClientData
	msg.Handler = call

	req := request{
		ctx:  ctx,
		msg:  msg,
		resp: make(chan response, 1),
	}
	c.reqCh <- req
	resp := <-req.resp

	return resp.msg, resp.err
}

func (c *Client) send() {
	id := uint64(0)
	for req := range c.reqCh {
		c.mu.Lock()
		c.responses[id] = req.resp
		c.mu.Unlock()

    // TODO(jdoak): I am so confused by this code.  Could not get the id thing
    // to work with the goroutines and now, if you remove the func(){} wrapper,
    // this think stops working.  If you leave it, it works.
    // MAKES NO SENSE!!!!!
		func() {
      defer log.Infof("func() done")
			for {
				select {
				case <-req.ctx.Done():
					c.mu.Lock()
					c.responses[id] <- response{err: req.ctx.Err()}
					delete(c.responses, id)
					c.mu.Unlock()
					return
				case <-c.done:
					return
				default:
					// Do nothing.
				}

				req.msg.ID = id
				if err := req.msg.Encode(c.inConn); err != nil {
					continue
				}
				return
			}
		}()
		id++
	}
}

// receive listens for responses from the server and routes them to the
// correct listeners.
func (c *Client) receive() {
	errCh := c.decode()
	for {
		select {
		case <-c.done:
			return
		case err := <-errCh:
			log.Error(err)
		}
	}
}

// decode handles decoding all messages coming from the server and sending them
// to the proper listener.
func (c *Client) decode() chan error {
	ch := make(chan error, 1)
	go func() {
		for {
			msg := &ServerMsg{}
			err := msg.Decode(c.outConn)
			if err != nil {
				if err == io.EOF {
					return // Socket is closed, no need to listen any longer.
				}
				go func() { ch <- err }()
				continue
			}
			go c.respond(msg, ch)
		}
	}()
	return ch
}

// respond returns the msg that came from the server to the proper listener
// or an error on "ch" if there is no proper listener.
func (c *Client) respond(msg *ServerMsg, ch chan error) {
	c.mu.Lock()
	respCh, ok := c.responses[msg.ID]
	c.mu.Unlock()

	if !ok {
		select {
		case ch <- fmt.Errorf("no return channel for msg ID %d", msg.ID):
		case <-time.After(30 * time.Second):
			panic("crap")
		}
	} else {
		select {
		case respCh <- response{msg: msg}:
		default:
			panic("the respCh blocked, which should never happen")
		}
	}
}
