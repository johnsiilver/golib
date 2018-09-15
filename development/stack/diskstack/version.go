package diskstack

import (
  "bytes"
  "encoding/binary"
  "encoding/gob"
  "fmt"
  "os"
)

// internalVersion tracks the current version of the disk representation that
// we read/write. We should be able to read back versions that are <= our
// version number.
const internalVersion = 1

// VersionInfo is used to encode version information for the disk stack into
// our files.  This struct can never have a field removed only added.
type VersionInfo struct {
	// Version is the version of encoding used to by stack to encode this.
	// This is not the same as the Semantic Version of the code.
	Version uint64
	// Created is when the file was actually created in Unix time.
	Created int64
}

// Encode encodes the VersionInfo to the start of a file. If an error returns
// the file will be truncated to 0.
func (v VersionInfo) encode(f *os.File) (n int, err error) {
	defer func() {
		if err != nil {
			f.Truncate(0)
			f.Seek(0, 0)
		}
		f.Sync()
	}()

	if _, err = f.Seek(0, 0); err != nil {
		return 0, fmt.Errorf("could not seek the beginning of the file: %s", err)
	}

	buff := bytes.NewBuffer([]byte{})
	enc := gob.NewEncoder(buff)

	// Write our data to the buffer.
	if err = enc.Encode(v); err != nil {
		return 0, fmt.Errorf("could not gob encode the data: %s", err)
	}

	// Write our version number to disk.
	b := make([]byte, int64Size)
	binary.LittleEndian.PutUint64(b, v.Version)
	if _, err = f.Write(b); err != nil {
		return 0, fmt.Errorf("unable to write the version number to disk")
	}

	f.Seek(-int64Size, 2)
	if _, err := f.Read(b); err != nil {
		panic(err)
	}

	// Write our size header to disk.
	l := buff.Len()
	binary.LittleEndian.PutUint64(b, uint64(l))
	if _, err = f.Write(b); err != nil {
		return 0, fmt.Errorf("unable to write data size to disk: %s", err)
	}

	f.Seek(-int64Size, 2)
	if _, err := f.Read(b); err != nil {
		panic(err)
	}

	// Write the buffer to disk.
	if _, err = f.Write(buff.Bytes()); err != nil {
		return 0, fmt.Errorf("unable to write our version info to disk: %s", err)
	}

	return int(int64Size + int64Size + l), nil
}

func (v *VersionInfo) decode(f *os.File) (int, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return 0, fmt.Errorf("could not seek the beginning of the file: %s", err)
	}

	// Read version number back.
	b := make([]byte, int64Size)
	if _, err := f.Read(b); err != nil {
		return 0, fmt.Errorf("cannot read the version number from the file: %s", err)
	}
	ver := binary.LittleEndian.Uint64(b)
	if ver > internalVersion {
		return 0, fmt.Errorf("cannot read this file: current version number %d is greater than the libraries version number %d", ver, internalVersion)
	}

	// Read version block size.
	if _, err := f.Read(b); err != nil {
		return 0, fmt.Errorf("cannot read the version block size back from the file: %s", err)
	}
	blockSize := binary.LittleEndian.Uint64(b)

	b = make([]byte, blockSize)
	if _, err := f.Read(b); err != nil {
		return 0, fmt.Errorf("cannot read the version block from the file: %s", err)
	}

	dec := gob.NewDecoder(bytes.NewBuffer(b))
	if err := dec.Decode(v); err != nil {
		return 0, fmt.Errorf("cannot convert the version block on disk into a VersionInfo struct: %s", err)
	}
	return int(blockSize + int64Size), nil
}
