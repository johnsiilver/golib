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

		if !bytes.Equal(val, v) {
			t.Errorf("a value was not correctly stored")
		}
	}

	if _, err := r.Read([]byte("helloworld")); err == nil {
		t.Errorf("a non-existant key passed to Read() did not return an error")
	}
}

func TestDiskMapDuplicateKeys(t *testing.T) {
	p := path.Join(os.TempDir(), nextSuffix())
	w, err := New(p)
	if err != nil {
		panic(err)
	}
	defer os.Remove(p)

	_1stKey := []byte(nextSuffix())
	_1stData := randStringBytes()
	dupKey := []byte(nextSuffix())
	dupData0 := randStringBytes()
	dupData1 := randStringBytes()
	_2ndKey := []byte(nextSuffix())
	_2ndData := randStringBytes()

	for _, kv := range []KeyValue{
		{Key: _1stKey, Value: _1stData},
		{Key: dupKey, Value: dupData0},
		{Key: _2ndKey, Value: _2ndData},
		{Key: dupKey, Value: dupData1},
	} {
		if err := w.Write(kv.Key, kv.Value); err != nil {
			t.Fatalf("error writing:\nkey:%q\nvalue:%q\n", kv.Key, kv.Value)
		}
	}

	w.Close()

	r, err := Open(p)
	if err != nil {
		t.Fatalf("error opening diskmap %q", err)
	}

	got, err := r.Read(dupKey)
	if err != nil {
		t.Fatalf("TestDiskMapDuplicateKeys(r.Read()): got err == %s, want err == nil", err)
	}
	if !bytes.Equal(got, dupData1) {
		t.Fatalf("TestDiskMapDuplicateKeys(r.Read()): got incorrect data")
	}

	gotBatch, err := r.ReadAll(dupKey)
	if err != nil {
		t.Fatalf("TestDiskMapDuplicateKeys(r.ReadAll()): got err == %s, want err == nil", err)
	}
	if len(gotBatch) != 2 {
		t.Fatalf("TestDiskMapDuplicateKeys(r.ReadAll()): got %d return values, want %d", len(gotBatch), 2)
	}
	want := [][]byte{dupData0, dupData1}
	for i := 0; i < len(gotBatch); i++ {
		if !bytes.Equal(gotBatch[i], want[i]) {
			t.Fatalf("TestDiskMapDuplicateKeys(r.ReadAll()): returned value %d was incorrect", i)
		}
	}
}

func BenchmarkDiskMapWriter(b *testing.B) {
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
