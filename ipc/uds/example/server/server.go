package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/johnsiilver/golib/ipc/uds"
)

func main() {
	socketAddr := filepath.Join(os.TempDir(), uuid.New().String())

	cred, _, err := uds.Current()
	if err != nil {
		panic(err)
	}

	// This will set the socket file to have a uid and gid of whatever the
	// current user is. 0770 will be set for the file permissions (though on some
	// systems the sticky bit gets set, resulting in 1770.
	serv, err := uds.NewServer(socketAddr, cred.UID.Int(), cred.GID.Int(), 0770)
	if err != nil {
		panic(err)
	}

	fmt.Println("Listening on socket: ", socketAddr)

	// This listens for a client connecting and returns the connection object.
	for conn := range serv.Conn() {
		conn := conn

		// We spinoff handling of this connection to its own goroutine and
		// go back to listening for another connection.
		go func() {
			// We are checking the client's user ID to make sure its the same
			// user ID or we reject it. Cred objects give you the user's
			// uid/gid/pid for filtering.
			if conn.Cred.UID.Int() != cred.UID.Int() {
				log.Printf("unauthorized user uid %d attempted a connection", conn.Cred.UID.Int())
				conn.Close()
				return
			}
			// Write to the stream every 10 seconds until the connection closes.
			for {
				if _, err := conn.Write([]byte(fmt.Sprintf("%s\n", time.Now().UTC()))); err != nil {
					conn.Close()
				}
				time.Sleep(10 * time.Second)
			}
		}()
	}
}
