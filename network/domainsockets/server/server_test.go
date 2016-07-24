package server

import (
  log "github.com/golang/glog"

  "bufio"
  "fmt"
  "os"
  "path"
  "net"
  "testing"

  "github.com/johnsiilver/golib/network/domainsockets/shared"
  "github.com/pborman/uuid"
)

const numMess = 1000000

func TestServer(t *testing.T) {
  uid := uuid.New()
  s, err := New(uid)
  if err != nil {
    panic(err)
  }

  s.Register(
    "HelloWorld",
    func(req *shared.ClientMsg) (*shared.ServerMsg, error){
      ret := &shared.ServerMsg{}
      if string(req.Data) != "Hello" {
        return ret, fmt.Errorf("did not receive Hello")
      }
      ret.ID = req.ID
      ret.Type = shared.ServerData
      ret.Data = []byte("World")
      return ret, nil
    },
  )

  s.Start()

  // Have the client start listening for responses from the server.
  clientUID := uuid.New()
  clientPath := path.Join(os.TempDir(), clientUID)
  outConn, err := net.ListenUnixgram(udsType, &net.UnixAddr{clientPath, udsType})
  if err != nil {
    panic(err)
  }

  // Dial the server.
  log.Infof("client: dialing the server")
  setupConn, err := net.DialUnix(
    udsType,
    nil,
    &net.UnixAddr{path.Join(os.TempDir(), uid), udsType},
  )
  if err != nil {
    panic(err)
  }

  // Send the server the socket we are listening on.
  log.Infof("client: sending uid to server")
  if err := shared.SetupEncode(clientUID, setupConn); err != nil {
    panic(err)
  }
  setupConn.Close()

  // Get the socket the server is going to listen on.
  out :=  bufio.NewReader(outConn)
  inUUID, err := shared.SetupDecode(out)
  if err != nil {
    panic(err)
  }
  log.Infof("client: received server uid for conn")

  // Dial the server.
  in, err := net.DialUnix(
    udsType,
    nil,
    &net.UnixAddr{path.Join(os.TempDir(), inUUID), udsType},
  )
  if err != nil {
    panic(err)
  }
  log.Infof("client: dialed server")

  go func() {
    for i := 0; i < numMess; i++ {
      for {
        to := shared.ClientMsg{ID: uint64(i), Type: shared.ClientData, Data: []byte("Hello"), Handler: "HelloWorld"}
        if err := to.Encode(in); err != nil {
          continue
        }
        break
      }
    }
  }()

  // JDOAK: we seem to have sent everything, but don't receive everything.
  found := map[uint64]bool{}
  for i :=0 ; i < numMess; i++ {
    srvResp := shared.ServerMsg{}
    for {
      if err := srvResp.Decode(out); err != nil {
        continue
      }
      break
    }

    if found[srvResp.ID] {
      t.Fatalf("found id %d more than once", srvResp.ID)
    }
    found[srvResp.ID] = true

    if string(srvResp.Data) != "World" {
      t.Fatalf("srvResp.Data was %s", string(srvResp.Data))
    }
  }

  if len(found) != numMess{
    t.Fatalf("number of msgs: got %d, want %d", len(found), numMess)
  }

}
