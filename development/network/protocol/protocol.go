// Package protocol implements a transport protocol similar to TCP for sending
// data across an io.Writer/io.Reader. As this is meant for communications
// between processes, it does not have acknowledgements or windowing.
// The application may need to format its own binary messaging on top of this,
// but this protocol will handling sharding the data into multiple packets.
package protocol

import (
  "bytes"
  "encoding/binary"
  "io"
)

const (
  Syn = 0
  Data = 1
  Fin = 3
)

type Packet struct {
  SrcPort, DstPort uint64
  Flag uint64
  Offset uint64
  Data []byte
}

func (p Packet)Encode(w io.Writer) error {
  buff := new(bytes.Buffer)
  i := make([]byte, 8)

  for _, x := range []uint64{p.Port, p.Flag, p.Offset} {
    binary.Write(buff, binary.BigEndian, x)
  }
  binary.Write(buff, binary.BigEndian, p.Data)
  s := buff.Len()

  if n, _ := w.Write(buff.Bytes()); n != s {
    return fmt.Errorf("expected to write %d bytes to stream, wrote %d", s, n)
  }
  return nil
}

func (p *Packet) Decode(r io.Reader) error {
  var err error

  for _, x := range []*uint64{&p.Port, &p.Flag, &p.Offset} {
    err = binary.Read(r, binary.BigEndian, x)
    if err != nil {
      return fmt.Errorf("return packet was corrupted")
    }
  }

  b := make([]byte, p.Offset)  // TODO(johnsiilver): Implement circular buffer.
  if n, _ := read.Read(p.Data); n != p.Offset {
    return fmt.Errorf("packet's data field was size %d, expected %d", n, p.Offset)
  }

  buf := bytes.NewBuffer(b)

  if err := binary.Read(buf, binary.BigEndian, &p.Data); err != nil {
    return fmt.Errorf("problem reading packets data back: %s", err)
  }
  return nil
}

// Send provides a method for sending Data to a destination port on the far
// side.  Two Send{} should NEVER share the same io.Writer object, as that will
// lead to duplicate srcPort, which will confuse the far end.
type Send struct {
  srcPort uint64
  mu sync.Mutex  // Protects everything above it.

  maxPacketSize uint64
  w io.Writer
}

// NewSend is the constructor for Send.
func NewSend(maxPacketSize int, w io.Writer) (*Send, error) {
  s := &Send{maxPacketSize: maxPacketSize, w: w}
  s.start()

  return s, nil
}

// Data sends the data to the dstPort on the other side of the connection.  This
// is thread-safe.
func (s *Send) Data(dstPort uint64, data []byte) error {
  s.mu.Lock()
  p := s.srcPort
  s.srcPort++
  s.mu.Unlock()

  p := Packet{
    Port: p,
    Flag:
  }
}
