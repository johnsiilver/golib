package rpc

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/johnsiilver/golib/ipc/uds"
	chunkRPC "github.com/johnsiilver/golib/ipc/uds/highlevel/chunk/rpc"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type Error struct {
	Msg string
}

func (e Error) AsError() error {
	if e.Msg == "" {
		return nil
	}
	return e
}
func (e Error) Error() string {
	return e.Msg
}

type SumReq struct {
	Ints []int
}

type SumResp struct {
	Sum int
	Err Error
}

type MathServer struct{}

func (m *MathServer) Sum(ctx context.Context, req []byte) ([]byte, error) {
	request := SumReq{}
	if err := json.Unmarshal(req, &request); err != nil {
		return nil, chunkRPC.Errorf(chunkRPC.ETBadData, "request could not be unmarshalled into SumReq: %s", err)
	}

	response := SumResp{}
	for _, i := range request.Ints {
		response.Sum += i
	}
	b, err := json.Marshal(response)
	if err != nil {
		return nil, chunkRPC.Errorf(chunkRPC.ETBadData, "request could not be unmarshalled into SumReq: %s", err)
	}
	return b, nil
}

func TestClientServer(t *testing.T) {
	// Get socket and user info.
	socketAddr := filepath.Join(os.TempDir(), uuid.New().String())

	cred, _, err := uds.Current()
	if err != nil {
		panic(err)
	}

	// Setup server.
	serv, err := NewServer(socketAddr, cred.UID.Int(), cred.GID.Int(), 0770)
	if err != nil {
		panic(err)
	}

	ms := MathServer{}

	serv.RegisterMethod("/math/sum", ms.Sum)

	go func() {
		if err := serv.Start(); err != nil {
			log.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Second)

	// Setup client
	client, err := New(socketAddr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 1770})
	if err != nil {
		panic(err)
	}

	wg := sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		i := i
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()

			req := SumReq{Ints: []int{rand.Int(), rand.Int(), rand.Int()}}
			resp := SumResp{}
		retry:
			if err := client.Call(ctx, "/math/sum", req, &resp); err != nil {
				if chunkRPC.Retryable(err) {
					// Okay, so you probably should do this in a for loop, but I wanted to use
					// a label for the hell of it.
					goto retry
				}
				t.Errorf("TestClientServer(interation %d): got err == %s, want err == nil,", i, err)
				return
			}
			if resp.Err.AsError() != nil {
				t.Errorf("TestClientServer(interation %d): got err == %s, want err == nil,", i, err)
				return
			}
			want := req.Ints[0] + req.Ints[1] + req.Ints[2]
			if resp.Sum != want {
				t.Errorf("TestClientServer(interation %d): got %d, want %d,", i, resp.Sum, want)
			}
		}()
	}

	wg.Wait()
}
