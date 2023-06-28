package file_v0

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/pierrec/lz4"
)

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes() []byte {
	b := make([]byte, 1000)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

func TestDiskList(t *testing.T) {
	ri := func(src io.Reader) io.ReadCloser {
		return ioutil.NopCloser(lz4.NewReader(src))
	}
	wi := func(dst io.Writer) io.WriteCloser {
		return lz4.NewWriter(dst)
	}

	cases := []struct {
		desc     string
		roptions []ReadOption
		woptions []WriteOption
	}{
		{"No options", nil, nil},
		{
			"Compression",
			[]ReadOption{ReadIntercept(ri)},
			[]WriteOption{WriteIntercept(wi)},
		},
		{"Caching", []ReadOption{CacheIndex()}, nil},
	}

	for _, c := range cases {
		t.Run(
			c.desc,
			func(t *testing.T) {
				SubDiskList(t, c.roptions, c.woptions)
			},
		)
	}
}

func SubDiskList(t *testing.T, readOptions []ReadOption, writeOptions []WriteOption) {
	p := path.Join(os.TempDir(), nextSuffix())
	w, err := New(p, writeOptions...)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)

	data := make([][]byte, 1000)
	for i := 0; i < 1000; i++ {
		v := randStringBytes()

		if err := w.Write(v); err != nil {
			t.Fatalf("error writing:\nkey:%q\nvalue:%q\n", i, v)
		}
		data[i] = v
	}

	if err := w.Close(); err != nil {
		t.Fatalf("error closing the Writer: %q", err)
	}

	r, err := Open(p, readOptions...)
	if err != nil {
		t.Fatalf("error opening diskmap %q", err)
	}

	for i, v := range data {
		val, err := r.Read(i)
		if err != nil {
			t.Fatalf("an index(%d)/value pair was lost: %q", i, err)
		}

		if bytes.Compare(val, v) != 0 {
			t.Fatalf("a value(@%d) was not correctly stored, got %s, want %s", i, string(val), string(v))
		}
	}

	for value := range r.Range(context.Background(), 0, -1) {
		if value.Err != nil {
			t.Fatalf("got unexpected error in Range(): %s", value.Err)
		}

		if bytes.Compare(value.Value, data[value.Index]) != 0 {
			t.Fatalf("during Range(): a value(@%d) was not correctly stored, got %s, want %s", value.Index, string(value.Value), string(data[value.Index]))
		}
	}
}

func BenchmarkDisksliceWrite(b *testing.B) {
	b.ReportAllocs()

	p := path.Join(os.TempDir(), nextSuffix())

	w, err := New(p)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)

	b.ResetTimer()
	for i := 0; i < 100000; i++ {
		v := randStringBytes()

		if err := w.Write(v); err != nil {
			b.Fatalf("error writing:\nkey:%q\nvalue:%q\n", i, v)
		}
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	b.StopTimer()
}

func nextSuffix() string {
	r := uint32(time.Now().UnixNano() + int64(os.Getpid()))

	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}
