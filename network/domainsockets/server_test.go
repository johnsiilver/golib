package domainsockets

import (
	log "github.com/golang/glog"
	"sort"
	"time"

	"fmt"

	"sync"
	"testing"

	"github.com/pborman/uuid"
	"golang.org/x/net/context"
)

const numMess = 100000

func TestServer(t *testing.T) {
	uid := uuid.New()
	s, err := NewServer(uid)
	if err != nil {
		panic(err)
	}

	s.Register(
		"HelloWorld",
		func(req *ClientMsg) (*ServerMsg, error) {
			ret := &ServerMsg{}
			if string(req.Data) != "Hello" {
				return ret, fmt.Errorf("did not receive Hello")
			}
			ret.ID = req.ID
			ret.Type = ServerData
			ret.Data = []byte("World")
			return ret, nil
		},
	)

	s.Start()

	client, err := DialServer(uid)
	if err != nil {
		t.Fatal(err)
	}

	args := &ClientMsg{Data: []byte("Hello")}
	handler := "HelloWorld"
	wg := &sync.WaitGroup{}
	mu := sync.Mutex{}
	messagesSent := map[int]bool{}
	for i := 0; i < numMess; i++ {
		messagesSent[i] = true
	}

	//wg.Add(numMess)
	for i := 0; i < numMess; i++ {
		wg.Add(1)
		go func(x int) {
			defer wg.Done()
			defer func() {
				mu.Lock()
				defer mu.Unlock()
				delete(messagesSent, x)
			}()
			resp, err := client.Call(context.Background(), handler, args)
			if err != nil {
				t.Fatal(err)
			}
			if string(resp.Data) != "World" {
				t.Fatalf("srvResp.Data was %s", string(resp.Data))
			}
		}(i)
	}
	go func() {
		for {
			mu.Lock()
			l := len(messagesSent)
			mu.Unlock()
			if l == 0 {
				return
			}
			vals := []int{}
			mu.Lock()
			for k := range messagesSent {
				vals = append(vals, k)
			}
			mu.Unlock()
			sort.Ints(vals)
			log.Infof("values still being waited on: %#v", vals)
			time.Sleep(5 * time.Second)
		}
	}()
	wg.Wait()

	/*
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
	  if err := SetupEncode(clientUID, setupConn); err != nil {
	    panic(err)
	  }
	  setupConn.Close()

	  // Get the socket the server is going to listen on.
	  out :=  bufio.NewReader(outConn)
	  inUUID, err := SetupDecode(out)
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
	        to := ClientMsg{ID: uint64(i), Type: ClientData, Data: []byte("Hello"), Handler: "HelloWorld"}
	        if err := to.Encode(in); err != nil {
	          continue
	        }
	        break
	      }
	    }
	  }()

	  found := map[uint64]bool{}
	  for i :=0 ; i < numMess; i++ {
	    srvResp := ServerMsg{}
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
	*/
}
