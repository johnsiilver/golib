package diskmap

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/johnsiilver/golib/diskmap/internal/ordered"
)

// OpenInMemory opens a Reader that reads the entire diskmap into memory.
// All lookups are done against an ordered in-memory map. Altering data that
// is returned with alter it in the map, which is unsafe. All data should be
// copied before altering. Use Clone() for this.
func OpenInMemory(p string) (Reader, error) {
	entireFile, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}

	f := bytes.NewReader(entireFile)

	h := header{}
	if err := h.read(f); err != nil {
		return nil, err
	}

	data := ordered.New[string, [][]byte]()

	if _, err := f.Seek(h.indexOffset, 0); err != nil {
		return nil, fmt.Errorf("cannot seek to index offset: %q", err)
	}

	indexes := make([]entry, h.num)
	for i := 0; i < int(h.num); i++ {
		var dOff, dLen, kLen int64

		if err := binary.Read(f, endian, &dOff); err != nil {
			return nil, fmt.Errorf("cannot read a data offset in index: %q", err)
		}

		if err := binary.Read(f, endian, &dLen); err != nil {
			return nil, fmt.Errorf("cannot read a key offset in index: %q", err)
		}

		if err := binary.Read(f, endian, &kLen); err != nil {
			return nil, fmt.Errorf("cannot read a key offset in index: %q", err)
		}

		key := make([]byte, kLen)
		_, err := io.ReadFull(f, key)
		if err != nil {
			return nil, fmt.Errorf("error reading in a key from the index: %q", err)
		}

		indexes[i] = entry{key: key, offset: dOff, length: dLen}
	}

	// Allocate all the buffer we need for the data in a single allocation.
	buffNeeded := h.indexOffset - reservedHeader
	buff := make([]byte, buffNeeded)

	for _, e := range indexes {
		if _, err := f.Seek(e.offset, 0); err != nil {
			return nil, fmt.Errorf("cannot seek to data offset: %q", err)
		}

		b := buff[:e.length]
		buff = buff[e.length:]

		if _, err := io.ReadFull(f, b); err != nil {
			return nil, fmt.Errorf("cannot read data: %q", err)
		}

		strKey := byteSlice2String(e.key)
		val, ok := data.Get(strKey)
		if !ok {
			data.Set(strKey, [][]byte{b})
			continue
		}
		data.Set(strKey, append(val, b))
	}

	return &memReader{data: data}, nil
}

type memReader struct {
	data *ordered.Map[string, [][]byte]
}

// Exists returns true if the key exists in the diskmap. Thread-safe.
func (r *memReader) Exists(key []byte) bool {
	_, ok := r.data.Get(byteSlice2String(key))
	return ok
}

// Keys returns all the keys in the diskmap. This reads only from memory
// and does not access disk. Thread-safe.
func (r *memReader) Keys(ctx context.Context) chan []byte {
	ch := make(chan []byte, 1)
	go func() {
		defer close(ch)
		for kv := range r.data.Range(ctx) {
			select {
			case <-ctx.Done():
				return
			case ch <- UnsafeGetBytes(kv.Key):
			}
		}
	}()
	return ch
}

// Read fetches key "k" and returns the value. If there are multi-key matches,
// it returns the last key added. Errors when key not found. Thread-safe.
func (r *memReader) Read(k []byte) ([]byte, error) {
	vals, ok := r.data.Get(byteSlice2String(k))
	if !ok {
		return nil, ErrKeyNotFound
	}
	v := vals[len(vals)-1]

	return v, nil
}

// ReadAll fetches all matches to key "k". Does not error if not found. Thread-safe.
func (r *memReader) ReadAll(k []byte) ([][]byte, error) {
	vals, ok := r.data.Get(byteSlice2String(k))
	if !ok {
		return nil, ErrKeyNotFound
	}

	return vals, nil
}

// Range allows iteration over all the key/value pairs stored in the diskmap. If not interating
// over all values, Cancel() or a timeout should be used on the Context to prevent a goroutine leak.
func (r *memReader) Range(ctx context.Context) chan KeyValue {
	ch := make(chan KeyValue, 1)
	go func() {
		defer close(ch)
		for kv := range r.data.Range(ctx) {
			for _, v := range kv.Value {
				select {
				case <-ctx.Done():
					return
				case ch <- KeyValue{Key: UnsafeGetBytes(kv.Key), Value: v}:
				}
			}
		}
	}()
	return ch
}

// Close closes the diskmap file.
func (r *memReader) Close() error {
	return nil
}

// Clone returns a copy of the byte slice. This is for use with a Reader opened with
// OpenInMemory(), where it is unsafe to alter the returned data.
func Clone(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
