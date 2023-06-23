package main

import (
	"log"
	"math/rand"
	"os"
	"path"

	"github.com/johnsiilver/golib/diskslice/diskslice2"
)

func main() {
	p0 := path.Join(os.TempDir(), "disksliceV0.slice")
	p1 := path.Join(os.TempDir(), "disksliceV1.slice")

	w0, err := diskslice2.New(p0, diskslice2.WriteV0())
	if err != nil {
		panic(err)
	}
	w1, err := diskslice2.New(p1)
	if err != nil {
		panic(err)
	}

	for i := 0; i < 10000000; i++ {
		v := randStringBytes()

		if err := w0.Write(v); err != nil {
			log.Fatalf("error writing:\nkey:%q\nvalue:%q\n", i, v)
		}
		if err := w1.Write(v); err != nil {
			log.Fatalf("error writing:\nkey:%q\nvalue:%q\n", i, v)
		}
	}
	if err := w0.Close(); err != nil {
		panic(err)
	}
	if err := w1.Close(); err != nil {
		panic(err)
	}

	log.Println("created: ", p0)
	log.Println("created: ", p1)
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes() []byte {
	b := make([]byte, 1000)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}
