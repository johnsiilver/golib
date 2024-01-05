package stream

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/johnsiilver/golib/ipc/uds"
	pb "github.com/johnsiilver/golib/ipc/uds/highlevel/proto/stream/internal/proto"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestStream(t *testing.T) {
	addr := filepath.Join(os.TempDir(), "uds_stream_test")
	go func() {
		server(addr)
		log.Println("Server done")
	}()

	time.Sleep(1 * time.Second)

	// This handles the client side.
	us, err := uds.NewClient(addr, os.Geteuid(), os.Getegid(), nil)
	if err != nil {
		t.Fatalf("uds.NewClient(): %s", err)
	}
	sc, err := New(us, setTestName("client"))
	if err != nil {
		t.Fatalf("New(): %s", err)
	}

	out := []int32{}
	done := make(chan struct{})
	go func() {
		defer log.Println("client goroutine done")
		defer close(done)

		for {
			msg := &pb.MsgOut{}
			err := sc.Read(context.Background(), msg)
			if err != nil {
				if err == io.EOF {
					log.Println("Client got a Read() EOF")
					return
				}
				panic(err)
			}

			out = append(out, msg.Id)
		}
	}()

	for i := 0; i < 10000; i++ {
		msg := &pb.MsgIn{Id: int32(i)}
		if err := sc.Write(context.Background(), msg); err != nil {
			t.Fatalf("Write(): %s", err)
		}
	}

	log.Println("before Client CloseSend()")
	sc.CloseSend(context.Background())
	log.Println("Client sent CloseSend()")
	<-done
	log.Println("after ")
	if len(out) != 10000 {
		t.Fatalf("len(out) = %d, want 10000", len(out))
	}

	for i := 0; i < 10000; i++ {
		if out[i] != int32(i+1) {
			t.Fatalf("out[%d] = %d, want %d", i, out[i], i+1)
		}
	}
}

type servConnHandler struct {
	client *Client
}

func (c *servConnHandler) handle() {
	for {
		msg := &pb.MsgIn{}
		err := c.client.Read(context.Background(), msg)
		if err != nil {
			if err == io.EOF {
				log.Println("Server got a Read() EOF")
				c.client.CloseSend(context.Background())
				log.Println("Server sent CloseSend()")
				return
			}
			log.Printf("servConnHandler.handle(): Read(): %s", err)
			panic(err)
		}

		// Note: MsgIn and MsgOut have the same wire format, so it doesn't matter that
		// we are using MsgIn here.
		msg.Id++
		if err := c.client.Write(context.Background(), msg); err != nil {
			panic(err)
		}
	}
}

func server(addr string) {
	us, err := uds.NewServer(addr, os.Geteuid(), os.Getegid(), 0o700)
	if err != nil {
		panic(err)
	}

	for conn := range us.Conn() {
		s, err := New(conn, setTestName("server"))
		if err != nil {
			panic(err)
		}

		go func() {
			sch := servConnHandler{s}
			sch.handle()
		}()
	}
}
