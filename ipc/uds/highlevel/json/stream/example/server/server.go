package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/johnsiilver/golib/ipc/uds"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/json/stream"
	"github.com/johnsiilver/golib/ipc/uds/highlevel/json/stream/example/messages"
)

func main() {
	socketAddr := filepath.Join(os.TempDir(), uuid.New().String())

	cred, _, err := uds.Current()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	udsServ, err := uds.NewServer(socketAddr, cred.UID.Int(), cred.GID.Int(), 0770)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Listening on socket: ", socketAddr)

	for conn := range udsServ.Conn() {
		conn := conn

		go func() {
			// We are checking the client's user ID to make sure its the same
			// user ID or we reject it. Cred objects give you the user's
			// uid/gid/pid for filtering.
			if conn.Cred.UID.Int() != cred.UID.Int() {
				log.Printf("unauthorized user uid %d attempted a connection", conn.Cred.UID.Int())
				conn.Close()
				return
			}

			// This is a summary message of the last 10 words we have received. We will
			// reuse this.
			sum := messages.Sum{
				Sum: make([]string, 0, 10),
			}

			// This wraps our conn in a stream client for reading/writing JSON.
			streamer, err := stream.New(conn)
			if err != nil {
				log.Println(err)
				conn.Close()
				return
			}

			// Receive 10 words from the client and then send back a list of the last 10
			// we got on this conn.
			for {
				m := messages.Word{}

				// Receive a message from the stream.
				if err := streamer.Read(&m); err != nil {
					if err != io.EOF {
						log.Println(err)
					}
					conn.Close()
					return
				}

				// Add the contained word to our summary.
				sum.Sum = append(sum.Sum, m.Word)

				// Sends back the sum if we have 10 words sent to us. We don't wait for
				// the write, we immediately start waiting for our next values.
				if len(sum.Sum) == 10 {
					sendSum := sum
					sum = messages.Sum{Sum: make([]string, 0, 10)}
					go func() {
						if err := streamer.Write(sendSum); err != nil {
							conn.Close()
							return
						}
					}()
				}
			}
		}()
	}
}
