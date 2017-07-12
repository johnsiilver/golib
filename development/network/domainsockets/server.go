/*
Package domainsockets prvoides for a unix domain socket server, similar to an RPC server
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
package domainsockets

import (
  log "github.com/golang/glog"
  "sort"

  "bufio"
  "fmt"
  "net"
  "os"
  "path"
  "sync"
  "time"

  "github.com/pborman/uuid"
)

const udsType = "unixgram"

// Handler provides a function that answers a request from a client and returns
// a response.  The ServerMsg returned must have the same ID as req.
type Handler func(req *ClientMsg) (*ServerMsg, error)

type register struct {
  handler Handler
  sync.Mutex
}

// Server provides a Unix Domain Socket procedure calling service.
type Server struct {
    setupConn *net.UnixConn
    registry map[string]*register
    bufferSize int
}

// New is the constructor for Server.
func NewServer(setupSocket string) (*Server, error) {
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

var setupBytes = make([]byte, LenTotal)

func (s *Server) listenSetup() {
  reader := bufio.NewReader(s.setupConn)

  for {
    uid, err := SetupDecode(reader)
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

  if err := out.SetWriteBuffer(5 * MiB); err != nil {
    log.Errorf("cannot set the Unix Domain Socket buffer to 5 MiB")
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
  if err := SetupEncode(uid, out); err != nil {
    out.Close()
    log.Infof("problem encoding UUIDv4 for setup: %s", err)
    return
  }

  go s.serveConn(in, out)
}

var (
    incoming = map[int]bool{}
    incomingMu = sync.Mutex{}
)

func (s *Server) serveConn(in, out *net.UnixConn) {
  defer in.Close()
  defer out.Close()

  go func() {
    for {
      vals := []int{}
      incomingMu.Lock()
      for k := range incoming{
        vals = append(vals, k)
      }
      incomingMu.Unlock()
      sort.Ints(vals)
      log.Infof("the server: values still being waited on: %#v", vals)
      time.Sleep(5 * time.Second)
    }
  }()

  inBuff := bufio.NewReader(in)
  for {
    msg := &ClientMsg{}

    select {
    case err := <-s.decoder(msg, inBuff):
      if err != nil {
        log.Errorf("problem with client message: %s", err)
        continue
      }
    case <-time.After(30 * time.Second):
      return
    }

    incomingMu.Lock()
    incoming[int(msg.ID)] = true
    incomingMu.Unlock()

    if msg.Type == ClientKeepAlive {
      log.Infof("%#v", msg)
      continue
    }

    log.Infof("server received msg.ID = %d", msg.ID)
    reg, ok := s.registry[msg.Handler]
    if !ok {
      log.Infof("can not locate Handler %q", msg.Handler)
      continue
    }

    go func(msg *ClientMsg, reg *register){
      srvMsg, err := reg.handler(msg)
      if err != nil {
        srvMsg.ID = msg.ID
        srvMsg.Data = []byte(fmt.Sprintf("handler(%s) error: %s", msg.Handler, err))
        reg.Lock()
        defer reg.Unlock()
        log.Infof("server: writing error response to client for ID: %d", msg.ID)
        if err := srvMsg.Encode(out); err != nil {
          log.Infof("cannot write to the client: %s", err)
        }
        return
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
      incomingMu.Lock()
      delete(incoming, int(msg.ID))
      incomingMu.Unlock()
    }(msg, reg)
  }
}

func (s *Server) decoder(msg *ClientMsg, r *bufio.Reader) <-chan error {
    ch := make(chan error, 1)

    go func(){
      if err := msg.Decode(r); err != nil {
        ch <-err
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
