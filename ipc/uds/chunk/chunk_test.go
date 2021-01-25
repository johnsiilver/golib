package chunk

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/kylelemons/godebug/pretty"
)

func TestClient(t *testing.T) {
	sendChunks := [][]byte{
		[]byte("hello world"),
		[]byte("are you ready to rock"),
		[]byte("i am"),
	}

	socketAddr := filepath.Join(os.TempDir(), uuid.New().String(), "socket")
	dirPath := filepath.Dir(socketAddr)
	if err := os.MkdirAll(dirPath, 0777); err != nil {
		panic(err)
	}
	defer func() {
		os.RemoveAll(dirPath)
	}()

	cred, _, err := uds.Current()
	if err != nil {
		panic(err)
	}

	udsServ, err := uds.NewServer(socketAddr, cred.UID.Int(), cred.UID.Int(), 0770)
	if err != nil {
		panic(err)
	}

	udsClient, err := uds.NewClient(socketAddr, cred.UID.Int(), cred.GID.Int(), []os.FileMode{0770, 1770})
	if err != nil {
		panic(err)
	}

	servConn := <-udsServ.Conn()

	serv, err := New(servConn)
	if err != nil {
		panic(err)
	}

	client, err := New(udsClient)
	if err != nil {
		panic(err)
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	// Server writes.
	go func() {
		defer wg.Done()
		for _, chunk := range sendChunks {
			if err := serv.Write(chunk); err != nil {
				panic(err)
			}
		}
	}()
	// Server reads.
	serverGot := [][]byte{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < len(sendChunks); i++ {
			buff, err := serv.Read()
			if err != nil {
				if err == io.EOF {
					return
				}
				panic(err)
			}
			serverGot = append(serverGot, buff.Bytes())
		}
	}()
	// Client writes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, chunk := range sendChunks {
			if err := client.Write(chunk); err != nil {
				panic(err)
			}
		}
	}()
	// Client reads.
	clientGot := [][]byte{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < len(sendChunks); i++ {
			buff, err := client.Read()
			if err != nil {
				if err == io.EOF {
					return
				}
				panic(err)
			}
			clientGot = append(clientGot, buff.Bytes())
		}
	}()

	wg.Wait()

	if diff := pretty.Compare(sendChunks, clientGot); diff != "" {
		t.Fatalf("TestClient(server receive): -want/+got:\n%s", diff)
	}

	if diff := pretty.Compare(sendChunks, serverGot); diff != "" {
		t.Fatalf("TestClient(server receive): -want/+got:\n%s", diff)
	}
}
