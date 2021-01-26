package uds

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kylelemons/godebug/pretty"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func TestUDS(t *testing.T) {
	socketAddr := filepath.Join(os.TempDir(), uuid.New().String())

	cred, _, err := Current()
	if err != nil {
		panic(err)
	}

	serv, err := NewServer(socketAddr, cred.UID.Int(), cred.GID.Int(), 0770)
	if err != nil {
		panic(err)
	}
	log.Println("server created")
	go func() {
		err := <-serv.Closed()
		if err != nil {
			panic(err)
		}
	}()

	var got []interface{}
	serverDone := make(chan struct{})
	currentConns := sync.WaitGroup{}
	go func() {
		defer close(serverDone)
		for conn := range serv.Conn() {
			log.Println("server had conn")
			conn := conn
			currentConns.Add(1)

			if diff := pretty.Compare(cred, conn.Cred); diff != "" {
				panic(fmt.Sprintf("-want/+got creds:\n%s", diff))
			}

			go func() {
				defer currentConns.Done()
				dec := json.NewDecoder(conn)
				m := Cred{}
				for {
					if err := dec.Decode(&m); err != nil {
						if err != io.EOF {
							got = append(got, err)
						}
						log.Println("server conn closed")
						return
					}
					log.Println("server received message")
					got = append(got, m)
					conn.Write([]byte("ack"))
				}
			}()
		}
	}()

	// Create client and send 10 messages.
	client, err := NewClient(socketAddr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 1770})
	if err != nil {
		panic(err)
	}
	log.Println("client created")

	credJSON, err := json.Marshal(cred)
	if err != nil {
		panic(err)
	}

	ackRecv := []byte("abcisaseasyas123") // Test we can read into a larger slice than the data.
	for i := 0; i < 10; i++ {
		ackRecv = ackRecv[:cap(ackRecv)]
		if _, err := client.Write(credJSON); err != nil {
			panic(err)
		}
		log.Println("client sent message")
		n, err := client.Read(ackRecv)
		if err != nil {
			log.Printf("error type %T", err)
			log.Printf("error stuff:\n%s", pretty.Sprint(err))
			panic(err)
		}
		ackRecv = ackRecv[:n]

		if string(ackRecv) != "ack" {
			panic(fmt.Sprintf("waiting for ack, got %q", string(ackRecv)))
		}
		log.Println("client got ack")
	}
	client.Close()
	log.Println("client done, waiting for conns to close")
	currentConns.Wait()
	log.Println("conns closed")

	if err := serv.Close(); err != nil {
		panic(err)
	}
	log.Println("server closed")

	want := []interface{}{}
	for i := 0; i < 10; i++ {
		want = append(want, cred)
	}

	if diff := pretty.Compare(want, got); diff != "" {
		t.Errorf("TestUDS: -want/+got:\n%s", diff)
	}
}
