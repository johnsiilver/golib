package main

import (
	"log"
	"math/rand"
	"os"
	"path"

	"github.com/johnsiilver/golib/diskmap"
)

func main() {
	p0 := path.Join(os.TempDir(), "diskmapV0.map")

	w0, err := diskmap.New(p0)
	if err != nil {
		panic(err)
	}

	for i := 0; i < 1000000; i++ {
		k := randStringBytes()
		v := randStringBytes()

		if err := w0.Write(k, v); err != nil {
			log.Fatalf("error writing:\nkey:%q\nvalue:%q\n", i, v)
		}
	}
	if err := w0.Close(); err != nil {
		panic(err)
	}

	log.Println("created: ", p0)
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes() []byte {
	b := make([]byte, 1000)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}
