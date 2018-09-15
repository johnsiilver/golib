package main

import (
	"encoding/gob"
	"flag"
	"io"
	"os"
	"path"

	"github.com/johnsiilver/golib/stack/diskstack"
	"github.com/pborman/uuid"
)

var (
	writer  = flag.String("writer", "", "The type of writer to use")
	num     = flag.Int("num", 0, "The number of files to write")
	sizeStr = flag.String("size", "", "The size of the entries: KB, MB, 10MB")
)

var files []string

const (
	KB = 1024
	MB = 1048576
)

func main() {
	flag.Parse()
	var rw io.ReadWriteSeeker
	switch *writer {
	case "osfile":
		rw = osFile()
	case "reverse":
		rw = reverseFile()
	default:
		panic("--writer is incorrect")
	}

	if *num <= 0 {
		panic("--num is incorrect")
	}

	var size int
	switch *sizeStr {
	case "KB":
		size = 1 * KB
	case "MB":
		size = 1 * MB
	case "10MB":
		size = 10 * MB
	default:
		panic("--size is incorrect")
	}

	data := make([]byte, size)
	enc := gob.NewEncoder(rw)
	for i := 0; i < *num; i++ {
		if err := enc.Encode(data); err != nil {
			panic(err)
		}
	}

	switch *writer {
	case "osfile":
		rw.Seek(0, 0)
	case "reverse":
		rw.Seek(0, 2)
	}

	dec := gob.NewDecoder(rw)

	for i := 0; i < *num; i++ {
		if err := dec.Decode(&data); err != nil {
			if err != io.EOF || i+1 != *num {
				panic(err)
			}
		}
	}

}

func osFile() *os.File {
	p := path.Join(os.TempDir(), uuid.New())
	f, err := os.OpenFile(p, os.O_CREATE+os.O_RDWR+os.O_EXCL, 0666)
	if err != nil {
		panic(err)
	}
	files = append(files, p)
	return f
}

func reverseFile() io.ReadWriteSeeker {
	f := osFile()
	return diskstack.Reverse{f}
}
