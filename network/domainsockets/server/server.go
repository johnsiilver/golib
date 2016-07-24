/*
Package server prvoides for a unix domain socket server, similar to an RPC server
like Grpc except that it is intended to only connect unix processes on the same device.

Usage

Usage is fairly simple:
  s, err := NewServer()
  if err != nil {
    // Do something
  }

  s.Register(header, handler)

  if err := s.Start(); err != nil {
    // Do something
  }
*/
package server

import (
  log "github.com/golang/glog"

  "bufio"
  "fmt"
  "net"
  "os"
  "path"
  "sync"
  "time"

  "github.com/johnsiilver/golib/network/domainsockets/shared"
  "github.com/pborman/uuid"
)

const udsType = "unixgram"

// Handler provides a function that answers a request from a client and returns
// a response.  The ServerMsg returned must have the same ID as req.
type Handler func(req *shared.ClientMsg) (*shared.ServerMsg, error)

type register struct {
  handler Handler
  sync.Mutex
}

// Server provides a Unix Domain Socket procedure calling service.
type Server struct {
    setupConn *net.UnixConn
    registry map[string]*register
}

// New is the constructor for Server.
func New(setupSocket string) (*Server, error) {
  p := path.Join(os.TempDir(), setupSocket)
  conn, err := net.ListenUnixgram(udsType, &net.UnixAddr{p, udsType})
  if err != nil {
    return nil, fmt.Errorf("could not listen on domain socket %q: %s", setupSocket, err)
  }

  s := &Server{setupConn: conn, registry: make(map[string]*register, 1)}

  return s, nil
}

func (s *Server) Start() {
    go s.listenSetup()
}

// Close closes all connections.  This server object cannot be reused.
func (s *Server) Close() {
  s.setupConn.Close()
}

var setupBytes = make([]byte, shared.LenTotal)

func (s *Server) listenSetup() {
  reader := bufio.NewReader(s.setupConn)

  for {
    uid, err := shared.SetupDecode(reader)
    if err != nil {
      log.Infof("problem reading conn setup request: %s", err)
      continue
    }
    log.Infof("server: received setup request on setup socket")
    go s.setupSockets(uid)
  }
}

// setupSockets sets up the sockets used to communicate between the client
// and the server.  If successful it spins up another goroutine to handle
// all communication from that client.
func (s *Server) setupSockets(remoteID string) {
  log.Infof("server: dialing client socket")
  out, err := net.DialUnix(
    udsType,
    nil,
    &net.UnixAddr{path.Join(os.TempDir(), remoteID), udsType},
  )
  if err != nil {
    log.Infof("problem dialing client's socket: %s", err)
    return
  }

  log.Infof("server: preparing to listen on new socket")
  uid := uuid.New()
  p := path.Join(os.TempDir(), uid)
  in, err := net.ListenUnixgram(udsType, &net.UnixAddr{p, udsType})
  if err != nil {
    out.Close()
    log.Infof("could not listen on domain socket %q: %s", p, err)
    return
  }

  log.Infof("server: sending a uid to the client")
  if err := shared.SetupEncode(uid, out); err != nil {
    out.Close()
    log.Infof("problem encoding UUIDv4 for setup: %s", err)
    return
  }

  go s.serveConn(in, out)
}

func (s *Server) serveConn(in, out *net.UnixConn) {
  defer in.Close()
  defer out.Close()

  inBuff := bufio.NewReader(in)
  received := 0
  for {
    msg := &shared.ClientMsg{}

    select {
    case err := <-s.decoder(msg, inBuff):
      if err != nil {
        log.Errorf("problem with client message: %s", err)
        continue
      }
    case <-time.After(30 * time.Second):
      return
    }

    if msg.Type == shared.ClientKeepAlive {
      log.Infof("%#v", msg)
      continue
    }
    received++
    log.Infof("received request %d", received)

    reg, ok := s.registry[msg.Handler]
    if !ok {
      log.Infof("can not locate Handler %q", msg.Handler)
      continue
    }

    go func(msg *shared.ClientMsg, reg *register){
      srvMsg, err := reg.handler(msg)
      if err != nil {
        srvMsg.ID = msg.ID
        srvMsg.Data = []byte(fmt.Sprintf("handler(%s) error: %s", msg.Handler, err))
        reg.Lock()
        defer reg.Unlock()
        log.Infof("server: writing error response to client for ID: %d", msg.ID)
        if err := srvMsg.Encode(out); err != nil {
          log.Infof("cannot write to the client: %s", err)
          return
        }
      }
      reg.Lock()
      defer reg.Unlock()
      log.Infof("server: writing response to client for ID: %d", msg.ID)
      for {
        if err := srvMsg.Encode(out); err != nil {
          log.Infof("cannot write to the client: %s", err)
          continue
        }
        break
      }
    }(msg, reg)
  }
}

func (s *Server) decoder(msg *shared.ClientMsg, r *bufio.Reader) <-chan error {
    ch := make(chan error, 1)

    go func(){
      if err := msg.Decode(r); err != nil {
        ch <-err
      }else{
        log.Infof("server: received request from client")
      }
      close(ch)
    }()

    return ch
}

// Register registers a handler.  This should only be called before Start(),
// otherwise the results are undetermined.
func (s *Server) Register(name string, h Handler) error {
  if _, ok := s.registry[name]; ok {
    return fmt.Errorf("cannot register name %q twice", name)
  }
  s.registry[name] = &register{handler: h}
  return nil
}
