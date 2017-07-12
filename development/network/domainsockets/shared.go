package domainsockets

/*
 TODO(johnsiilver): At some point, we should divide ClientMsg into multiple
 structs, Header, TLV, Handler and Message.  Each should have its own
 Marshal/Unmarshal methods.  Message would handle calling all the other
 Marshal methods.  This would allow for us to pass options (TLV) and have
 cleaner code.
*/

import (
	log "github.com/golang/glog"

	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
)

// MiB is a Mebibyte (sometimes called a Megabyte).
const (
  MiB = 1048576
  chunkSize = 5 * MiB
)

var (
	KeepAlive = byte(011)
	LenTotal  = 36
)

var (
	uuidRE = regexp.MustCompile(`[0-9a-f-]{36}`)
)

// SetupEncode encodes the uid (a UUIDv4 ID) into a message to be sent
//for setup.  This ID is the socket name inside os.TempDir() that the process is
// listening on.
func SetupEncode(uid string, w io.Writer) error {
	if !uuidRE.MatchString(uid) {
		return fmt.Errorf("name must be a UUIDv4 identifier")
	}
	w.Write([]byte(uid))

	return nil
}

// SetupDecode decodes a setup message into a UUIDv4 identifer. This ID is the
// unix socket in os.TempDir that the remote side is listening on.
func SetupDecode(r io.Reader) (string, error) {
	b := make([]byte, LenTotal)

	if n, _ := r.Read(b); n != LenTotal {
		return "", fmt.Errorf("did not receive the correct amount of info, got %d, want %d", n, LenTotal)
	}
	if !uuidRE.Match(b) {
		return "", fmt.Errorf("did not contain a valid UUIDv4")
	}
	return string(b), nil
}

const (
	// ClientKeepAlive indicates that the client is still alive.
	ClientKeepAlive = 0

	// ClientData indicates that the client is making a request to the server.
	ClientData = 1
)

const clientMsgHeader = 40

// ClientMsg represents the data to be sent on the wire from the client to
// the server.
type ClientMsg struct {
	ID      uint64
	Type    uint64
	Data    []byte
	Handler string
}

// Encode encodes the message in the following []byte format:
// [TotalSize][ID][Type][handler length][handler][data length][data]
func (m *ClientMsg) Encode(w io.Writer) error {
	// TODO(johnsiilver): We might get a speed increase if we use a circular
	// buffer for these values instead of creating them.
	id := make([]byte, 8)
	typ := make([]byte, 8)
	handleLen := make([]byte, 8)
	dataLen := make([]byte, 8)
	callSize := make([]byte, 8)

	// Write the ID of the call.
	n := binary.PutUvarint(id, m.ID)
	id = id[:n]

	// Write the Type of the call.
	n = binary.PutUvarint(typ, m.Type)
	typ = typ[:n]

	// Write length of handler.
	n = binary.PutUvarint(handleLen, uint64(len(m.Handler)))
	handleLen = handleLen[:n]

	// Write length of data.
	n = binary.PutUvarint(dataLen, uint64(len(m.Data)))
	dataLen = dataLen[:n]

	// Write the size of the call.
	n = binary.PutUvarint(callSize, uint64(len(id)+len(typ)+len(handleLen)+len(m.Handler)+len(dataLen)+len(m.Data)))
	callSize = callSize[:n]

	buf := bytes.NewBuffer(make([]byte, 0, 100))

	// Write everything to the buffer.
	buf.Write(callSize)
	buf.Write(id)
	buf.Write(typ)
	buf.Write(handleLen)
	buf.WriteString(m.Handler)
	buf.Write(dataLen)


	// Write the header buffer to output stream.
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("problem encoding ClientMsg onto io.Writer: %s", err)
	}

  if len(m.Data) > 0 {
    lBound := 0
    rBound := chunkSize
    if len(m.Data) < chunkSize {
      rBound = len(m.Data)
    }

    log.Infof("size of data to encode: %d", len(m.Data))
    for {
      log.Infof("writing chunk %d:%d", lBound, rBound)
      if _, err := w.Write(m.Data[lBound:rBound]); err != nil {
        return fmt.Errorf("problem encoding ClientMsg data onto io.Writer: %s", err)
      }
      if rBound == len(m.Data){
        break
      }
      lBound = rBound
      if rBound + chunkSize > len(m.Data){
        rBound = len(m.Data)
      }else{
        rBound += chunkSize
      }
    }
  }

	return nil
}

// Decodes a byte encoded message into Message.
func (m *ClientMsg) Decode(r io.ByteReader) error {
	// Read the message length.
	_, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("bytes did not contain an total length")
	}

	// Read the message ID.
	i, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("bytes did not contain an ID")
	}
	m.ID = i

	// Read the message Type.
	i, err = binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("bytes did not contain a Type: %s", err)
	}
	if i > ClientData {
		return fmt.Errorf("Type was invalid %d", i)
	}
	m.Type = i

	// Read the handler length.
	i, err = binary.ReadUvarint(r)
	if err != nil {
		log.Info(err)
		return fmt.Errorf("bytes did not contain handler length: %s", err)
	}

	// Read the handler back.
	by := make([]byte, i)
  for x := 0; x < int(i); x++ {
    by[x], err = r.ReadByte()
    if err != nil {
      return fmt.Errorf("handler was not encoded up to the length, expected %d, got %d bytes", i, x)
    }
  }
	m.Handler = string(by)
  log.Infof("handler was: %s", m.Handler)

	// Read the length of the data and the data back.
	i, err = binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("bytes did not contain data length")
	}

  log.Infof("i was %d", i)
  out := make([]byte, int(i))
  if i > 0 {
    for x := 0; x < int(i); x++ {
      out[x], err = r.ReadByte()
      if err != nil {
        continue
      }
    }
    /*
    lBound := 0
    rBound := chunkSize
    if i < chunkSize {
      rBound = int(i)
    }

    by = make([]byte, chunkSize)
    for {
      by = by[0: rBound - lBound]
      _, err = buf.Read(by)
    	if err != nil {
    		return fmt.Errorf("bytes was not encoded up to the length")
      }
      out.Write(by)

      if rBound == len(m.Data){
        break
      }
      lBound = rBound
      if rBound + chunkSize > int(i) {
        rBound = int(i)
      }else{
        rBound += chunkSize
      }
    }
    */
  }

	m.Data = out

	return nil
}

const (
	// ServerError indicates there was an error and that Data will be a string
	// message of why.
	ServerError = 0

	// ServerData indicates that the server is returning data.
	ServerData = 1
)

// ServerMsg represents a message returned by the message handler.
type ServerMsg struct {
	ID   uint64
	Type uint64
	Data []byte
}

const serverMsgHeader = 32

// Encode encodes the message in the following []byte format:
// [TotalLen][ID][Type][data length][data]
func (m *ServerMsg) Encode(w io.Writer) error {
	id := make([]byte, 8)
	typ := make([]byte, 8)
	dataLen := make([]byte, 8)
	size := make([]byte, 8)

	log.Infof("encoding msg %d", m.ID)
	// Write the ID of the call.
	n := binary.PutUvarint(id, m.ID)
	id = id[:n]

	// Write the type.
	n = binary.PutUvarint(typ, m.Type)
	typ = typ[:n]

	//Write length of data and then data.
	n = binary.PutUvarint(dataLen, uint64(len(m.Data)))
	dataLen = dataLen[:n]

	// Write the size of the call.
	n = binary.PutUvarint(size, uint64(len(id)+len(typ)+len(dataLen)+len(m.Data)))
	size = size[:n]

	buf := bytes.NewBuffer(make([]byte, 0, 100))
	buf.Write(size)
	buf.Write(id)
	buf.Write(typ)
	buf.Write(dataLen)
	buf.Write(m.Data)

	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("problem encoding ServerMsg onto io.Writer: %s", err)
	}

	return nil
}

// Decodes a byte encoded message into Message.
func (m *ServerMsg) Decode(r io.ByteReader) error {
	// Read the message length.
	size, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("bytes did not contain an total length")
	}

	b := make([]byte, int(size))
	n, _ := r.(io.Reader).Read(b)
	if n != int(size) {
		return fmt.Errorf("clientMsg could not read the total data sent back")
	}

	buf := bufio.NewReader(bytes.NewBuffer(b))

	// Read the message ID.
	i, err := binary.ReadUvarint(buf)
	if err != nil {
		return fmt.Errorf("bytes did not contain an ID")
	}
	m.ID = i

	// Read the Type.
	i, err = binary.ReadUvarint(buf)
	if err != nil {
		return fmt.Errorf("bytes did not contain the message type")
	}
	if i > ServerData {
		return fmt.Errorf("message type %d is not valid", i)
	}
	m.Type = i

	// Read the length of the data and the data.
	i, err = binary.ReadUvarint(buf)
	if err != nil {
		return fmt.Errorf("bytes did not contain data length")
	}

	by := make([]byte, i)
	n, _ = buf.Read(by)
	if n != int(i) {
		return fmt.Errorf("bytes was not encoded up to the length")
	}
	m.Data = by

	return nil
}
