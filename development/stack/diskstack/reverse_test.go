package diskstack

import (
	"bytes"
	"encoding/gob"
	"io"
	"os"
	"path"
	"testing"

	"github.com/pborman/uuid"
)

func TestRead(t *testing.T) {
	const (
		want = "hello world"
		data = "dlrow olleh"
	)

	// Create a file to stream in and out of.
	p := path.Join(os.TempDir(), uuid.New())
	f, err := os.OpenFile(p, os.O_CREATE+os.O_RDWR+os.O_EXCL, 0666)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)
	defer f.Close()

	f.Write([]byte(data))

	r := Reverse{f}

	buff := bytes.NewBuffer(nil)
	b := make([]byte, 4)
	for {
		n, err := r.Read(b)
		buff.Write(b[:n])
		if err != nil {
			if err != io.EOF {
				panic(err)
			}
			break
		}
	}

	got := buff.String()
	if got != want {
		t.Errorf("TestRead: got %s, want %s", got, want)
	}
}

func TestWrite(t *testing.T) {
	const (
		data = "hello world"
	)

	// Create a file to stream in and out of.
	p := path.Join(os.TempDir(), uuid.New())
	f, err := os.OpenFile(p, os.O_CREATE+os.O_RDWR+os.O_EXCL, 0666)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)
	defer f.Close()

	r := Reverse{f}

	if _, err := r.Write([]byte(data)); err != nil {
		panic(err)
	}

	buff := bytes.NewBuffer(nil)
	b := make([]byte, 2)
	for {
		n, err := r.Read(b)
		buff.Write(b[:n])
		if err != nil {
			if err == io.EOF {
				break
			} else {
				panic(err)
			}
		}
	}

	got := buff.String()
	if got != data {
		t.Errorf("TestWrite: got %s, want %s", got, data)
	}
}

func TestReverseWithGob(t *testing.T) {
	p := path.Join(os.TempDir(), uuid.New())
	f, err := os.OpenFile(p, os.O_CREATE+os.O_RDWR+os.O_EXCL, 0666)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)
	defer f.Close()

	data := "hello my baby, hello my darling, hello my ragtime gal"

	// Now wrap our data in our custom io.ReadWriteSeeker.
	r := Reverse{f}

	// Encode our data, not reversed, into a buffer.
	enc := gob.NewEncoder(r)
	if err := enc.Encode(data); err != nil {
		panic(err)
	}

	// Read it back.

	// Try to decode what we got back.
	var got string
	dec := gob.NewDecoder(r)
	if err := dec.Decode(&got); err != nil {
		panic(err)
	}

	// Make sure its what we wanted.
	if data != got {
		t.Fatalf("TestReverse: got %q, want %q", got, data)
	}
}

var files []string

const (
	KB = 1024
	MB = 1048576
)

func BenchmarkReadWrite(b *testing.B) {
	tests := []struct {
		desc     string
		rw       io.ReadWriteSeeker
		readSeek []int
		num      int
		size     int
	}{
		{"os.File-1000entries-1KB", osFile(), []int{0, 0}, 1000, KB},
		{"os.File-1000entries-1MB", osFile(), []int{0, 0}, 1000, MB},
		{"os.File-1000entries-10MB", osFile(), []int{0, 0}, 1000, 10 * MB},
		{"reverse-1000entries-1KB", reverseFile(), []int{0, 2}, 1000, KB},
		{"reverse-1000entries-1MB", reverseFile(), []int{0, 2}, 1000, MB},
		{"reverse-1000entries-10MB", reverseFile(), []int{0, 2}, 1000, 10 * MB},
	}

	for _, test := range tests {
		data := make([]byte, test.size)
		enc := gob.NewEncoder(test.rw)
		b.Run(
			test.desc,
			func(b *testing.B) {
				for i := 0; i < test.num; i++ {
					if err := enc.Encode(data); err != nil {
						b.Fatalf("benchmark %s: %s", test.desc, err)
					}
				}
				b.StopTimer()
				// Put the offset at wherever we should start reading form.
				test.rw.Seek(int64(test.readSeek[0]), test.readSeek[1])
				_, err := test.rw.Seek(0, 1)
				if err != nil {
					b.Fatal(err)
				}
				//glog.Infof("at offset: %d", n)
				dec := gob.NewDecoder(test.rw)
				b.StartTimer()
				for i := 0; i < test.num; i++ {
					if err := dec.Decode(&data); err != nil {
						if err != io.EOF || i+1 != test.num {
							b.Fatalf("benchmark %s: iteration %d: %s", test.desc, i, err)
						}
					}
				}
			},
		)
	}

	for _, p := range files {
		defer os.Remove(p)
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
	return Reverse{f}
}
