package shared

import (
  log "github.com/golang/glog"

  "bufio"
  "bytes"
  "encoding/binary"
  "fmt"
  "io"
  "regexp"
)

var (
  KeepAlive = byte(011)
  LenTotal = 36
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

// ClientMsg represents the data to be sent on the wire from the client to
// the server.
type ClientMsg struct {
  ID uint64
  Type uint64
  Data []byte
  Handler string
}

// Encode encodes the message in the following []byte format:
// [TotalSize][ID][Type][handler length][handler][data length][data]
func (m *ClientMsg) Encode(w io.Writer) error {
  buf := new(bytes.Buffer)

  // Write the ID of the call.
  l := make([]byte, 8)
  n := binary.PutUvarint(l, m.ID)
  buf.Write(l[:n])

  // Write the Type of the call.
  n = binary.PutUvarint(l, m.Type)
  buf.Write(l[:n])

  // Write length of handler and then handler.
  n = binary.PutUvarint(l, uint64(len(m.Handler)))
  buf.Write(l[:n])
  buf.WriteString(m.Handler)

  //Write length of data and then data.
  n = binary.PutUvarint(l, uint64(len(m.Data)))
  buf.Write(l[:n])
  buf.Write(m.Data)

  // Write the size of the call.
  bufBytes := buf.Bytes()
  n = binary.PutUvarint(l, uint64(len(bufBytes)))
  if _, err := w.Write(append(l[:n], bufBytes...)); err != nil {
    return fmt.Errorf("problem encoding ClientMsg onto io.Writer: %s", err)
  }

  return nil
}

// Decodes a byte encoded message into Message.
func (m *ClientMsg) Decode(r io.ByteReader) error {
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

  // Read the message Type.
  i, err = binary.ReadUvarint(buf)
  if err != nil {
    return fmt.Errorf("bytes did not contain a Type: %s", err)
  }
  if i > ClientData {
    return fmt.Errorf("Type was invalid %d", i)
  }
  m.Type = i

  // Read the handler length.
  i, err = binary.ReadUvarint(buf)
  if err != nil {
    log.Info(err)
    return fmt.Errorf("bytes did not contain handler length: %s", err)
  }

  // Read the handler back.
  by := make([]byte, i)
  n, _ = buf.Read(by)
  if n != int(i) {
    return fmt.Errorf("handler was not encoded up to the length, expected %d, got %d bytes", i, n)
  }
  m.Handler = string(by)

  // Read the length of the data and the data back.
  i, err = binary.ReadUvarint(buf)
  if err != nil {
    return fmt.Errorf("bytes did not contain data length")
  }

  by = make([]byte, i)
  n, _ = buf.Read(by)
  if n != int(i) {
    return fmt.Errorf("bytes was not encoded up to the length")
  }
  m.Data = by

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
  ID uint64
  Type uint64
  Data []byte
}

// Encode encodes the message in the following []byte format:
// [TotalLen][ID][Type][data length][data]
func (m *ServerMsg) Encode(w io.Writer) error {
  buf := new(bytes.Buffer)
  size := 0

  // Write the ID of the call.
  l := make([]byte, 8)
  n := binary.PutUvarint(l, m.ID)
  buf.Write(l[:n])
  size += n

  // Write the type.
  n = binary.PutUvarint(l, m.Type)
  buf.Write(l[:n])
  size += n

  //Write length of data and then data.
  n = binary.PutUvarint(l, uint64(len(m.Data)))
  buf.Write(l[:n])
  buf.Write(m.Data)
  size += n + len(m.Data)

  // Write the size of the call.
  n = binary.PutUvarint(l, uint64(size))

  if _, err := w.Write(append(l[:n], buf.Bytes()...)); err != nil {
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
