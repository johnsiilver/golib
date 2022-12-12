package diskmap

import (
	"bytes"
	"math/rand"
	"os"
	"path"
	"strconv"
	"testing"
	"time"
)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes() []byte {
	b := make([]byte, 1000)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

func TestDiskMap(t *testing.T) {
	p := path.Join(os.TempDir(), nextSuffix())
	w, err := New(p)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)

	data := make(map[string][]byte, 1000)
	for i := 0; i < 1000; i++ {
		k := []byte(nextSuffix())
		v := randStringBytes()

		if err := w.Write(k, v); err != nil {
			t.Fatalf("error writing:\nkey:%q\nvalue:%q\n", k, v)
		}
		data[string(k)] = v
	}

	if err := w.Close(); err != nil {
		t.Fatalf("error closing the Writer: %q", err)
	}

	r, err := Open(p)
	if err != nil {
		t.Fatalf("error opening diskmap %q", err)
	}

	for k, v := range data {
		val, err := r.Read([]byte(k))
		if err != nil {
			t.Errorf("a key/value pair was lost: %q", err)
			continue
		}

		if bytes.Compare(val, v) != 0 {
			t.Errorf("a value was not correctly stored")
		}
	}

	if _, err := r.Read([]byte("helloworld")); err == nil {
		t.Errorf("a non-existant key passed to Read() did not return an error")
	}
}

func BenchmarkDiskMap(b *testing.B) {
	b.ReportAllocs()

	p := path.Join(os.TempDir(), nextSuffix())
	w, err := New(p)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)

	b.ResetTimer()
	for i := 0; i < 10000; i++ {
		k := []byte(nextSuffix())
		v := randStringBytes()

		if err := w.Write(k, v); err != nil {
			b.Fatalf("error writing:\nkey:%q\nvalue:%q\n", k, v)
		}
	}

	if err := w.Close(); err != nil {
		b.Fatalf("error closing the Writer: %q", err)
	}
}

func nextSuffix() string {
	r := uint32(time.Now().UnixNano() + int64(os.Getpid()))

	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}
