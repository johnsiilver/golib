package domainsockets

import (
  "bytes"
  "encoding/binary"
  "fmt"
  "io"
  "unsafe"

  log "github.com/golang/glog"
)

// Call represents the input/output for a procedure calls. If input, Data
// represents the input arguments. If output, it represents the return data.
type Call struct {
  ID uint64
  Handler string
  Data *bytes.Buffer
}

// Handler provides a function that answers a request from a client and returns
// a response.
type Handler func(req *Call) (*Call)

const (
  MiB = 1048576
  chunkSize = 5 * MiB
)

// Circular buffers that reduce our allocations when processing packets.
var (
  buffers = make(chan *bytes.Buffer, 20)
  uvarBuffs = make(chan []byte, 20)
)

// getBuffs returns buffers from our circular buffers.  *bytes.Buffer is
// guarenteed to have no data.
func getBuffs() (*bytes.Buffer, []byte) {
  var (
    bb *bytes.Buffer
    b []byte
  )

  select {
  case bb = <-buffers:
  default:
    bb = bytes.NewBuffer(make([]byte, 5 *MiB))
  }

  select {
  case b = <-uvarBuffs:
  default:
    b = make([]byte, 8)
  }
  return bb, b
}

// putBuffs puts our various buffers back into the ciruclar buffers, unless
// the buffers are full, in which case they are released.
func putBuffs(bb *bytes.Buffer, b []byte) {
    bb.Reset()

    select {
    case buffers <-bb:
    default:
    }
    select {
    case uvarBuffs <-b:
    default:
    }
}

// packet encodes/decodes a packet onto the wire.  Packets should normally be
// created by helper functions and not used directly.
type packet struct {
  id uint64
  tlvSize uint64
  tlvs tlvs
}

// addTLV adds a TLV to the packet.  Not thread safe.
func (p *packet) addTLV(t tlv) {
   i := unsafe.Sizeof(t)
   p.tlvSize += uint64(i)

   p.tlvs = append(p.tlvs, t)
}

// encode encodes the Packet into a stream.  Not thread safe.
func (p *packet) encode(w io.Writer) error {
  if 16+p.tlvSize > 5 *MiB {
    return fmt.Errorf("Packet cannot exceed 5 MiB in size")
  }

  bb, b := getBuffs()
  defer putBuffs(bb, b)

  // Write id.
  binary.PutUvarint(b, p.id)
  bb.Write(b)

  // Write TLV size.
  binary.PutUvarint(b, p.tlvSize)
  bb.Write(b)

  if err := p.tlvs.encode(bb); err != nil {
    return err
  }

  out := bb.Bytes()
  n, _ := w.Write(out)
  if n != len(out) {
    return fmt.Errorf("packet encoding error: should have written %d bytes, wrote %d", len(out), n)
  }

  return nil
}

// decode decodes output on a stream into a packet.  Not thread safe.
func (p *packet) decode(r io.Reader) error {
  var err error

  br := r.(io.ByteReader)

  bb, b := getBuffs()
  defer putBuffs(bb, b)

  p.id, err = binary.ReadUvarint(br)
  if err != nil {
    return fmt.Errorf("packet did not contain ID")
  }

  p.tlvSize, err = binary.ReadUvarint(br)
  if err != nil {
    return fmt.Errorf("packet did not contain TLV size")
  }

  if n, err := io.CopyN(bb, r, int64(p.tlvSize)); err != nil {
    return fmt.Errorf("packet expected %d bytes, but only received %d", p.tlvSize, n)
  }

  p.tlvs = make(tlvs, 0, 1)
  if err := p.tlvs.decode(p.tlvSize, bb); err != nil {
    return fmt.Errorf("packet had problem decoding TLVs: %s", err)
  }

  return nil
}

type tlvs []tlv

func (t *tlvs) encode(w io.Writer) error {
  b := make([]byte, 16 + 4 *MiB) // Data is no larger that 4MiB + the header.

  for _, tlv := range *t {
    dstart := 0
    n := binary.PutUvarint(b, tlv.Type)
    dstart += n

    n = binary.PutUvarint(b[n:], tlv.Length)
    dstart += n
    if n := copy(b[dstart:], tlv.Value); n != int(tlv.Length) {
      panic(fmt.Sprintf("copied %d bytes, should have copied %d", n, tlv.Length))
    }
  }
  return nil
}

func (t *tlvs) decode(size uint64, r io.Reader) error {
  var read = 0
  var err error
  br := r.(io.ByteReader)
  for ;read < int(size); {
    tlv := tlv{}
    tlv.Type, err = binary.ReadUvarint(br)
    if err != nil {
      return err
    }
    read += binary.Size(tlv.Type)

    tlv.Length, err = binary.ReadUvarint(br)
    if err != nil {
      return err
    }
    read += binary.Size(tlv.Length)

    tlv.Value = make([]byte, int(tlv.Length))
    n, _ := r.Read(tlv.Value)
    if n != int(tlv.Length) {
      return fmt.Errorf("not enough data for TLV entry")
    }
    read += n
    *t = append(*t, tlv)
  }
  return nil
}

// Various tlv types.
const (
    DataPacket uint64 = 0
    DataClose uint64 = 1
    DataHandler uint64 = 2
    KeepAlive uint64 = 3
)

type tlv struct {
  Type uint64
  Length uint64
  Value []byte
}

// writeCall writes a Call{} onto the stream in however many packets it takes.
func writeCall(c *Call, w io.Writer) error {
  // If we are sure we can fit the header and data together.
  if binary.Size(c.ID) + len([]byte(c.Handler)) + c.Data.Len() <= 4 *MiB {
    p := packet{id: c.ID}
    p.addTLV(
      tlv{
        Type: DataHandler,
        Length: uint64(len(c.Handler)),
        Value: []byte(c.Handler),
      },
    )
    p.addTLV(
      tlv{
        Type: DataClose,
        Length: uint64(c.Data.Len()),
        Value: c.Data.Bytes(),
      },
    )
    if err := p.encode(w); err != nil {
      return fmt.Errorf("could not encode packet on to stream: %s", err)
    }
    return nil
  }

  // Send the header in its own packet.
  p := packet{id: c.ID}
  p.addTLV(
    tlv{
      Type: DataHandler,
      Length: uint64(len(c.Handler)),
      Value: []byte(c.Handler),
    },
  )
  if err := p.encode(w); err != nil {
    return fmt.Errorf("could not encode header packet on to stream: %s", err)
  }

  // Now send all the data in 4 MiB chunks.
  b := make([]byte, 4 * MiB)
  typ := DataPacket
  for {
    p.tlvs = nil
    n, err := c.Data.Read(b)
    if err != nil {
      typ = DataClose
    }
    p.addTLV(
      tlv{
        Type: typ,
        Length: uint64(n),
        Value: b[:n],
      },
    )
    if err := p.encode(w); err != nil {
      return fmt.Errorf("could not encode packet on to stream: %s", err)
    }
    if typ == DataClose {
      return nil
    }
  }
}

// callHandler reads packets off a connection, reassembles them, sends them
// to a handler, and sends the server responses on a channel.
type callHandler struct {
  conn io.Reader
  calls map[uint64]*Call
  handlers map[string]Handler
  responses chan *Call
  done chan struct{}
}

func newCallHandler(conn io.Reader, handlers map[string]Handler) (callHandle *callHandler, done chan struct{})  {
  done = make(chan struct{})
  callHandle = &callHandler{
    conn: conn,
    calls: make(map[uint64]*Call, 100),
    responses: make(chan *Call, 1000),
    done: done,
  }
  go callHandle.read()
  return
}

func (c *callHandler) read() {
  defer close(c.done)
  p := packet{}
  for {
    if err := p.decode(c.conn); err != nil {
      log.Infof("bad packet for decode: %s",err)
      return
    }
    for _, tlv := range p.tlvs {
      switch tlv.Type{
      case KeepAlive:
        // TODO(johnsiilver): Handle this.
      case DataHandler:
        c.calls[p.id] = &Call{
          ID: p.id,
          Handler: string(tlv.Value),
        }
      case DataPacket:
        v, ok := c.calls[p.id]
        if !ok {
          log.Infof("data packet came in before header")
          return
        }
        v.Data.Write(tlv.Value)
      case DataClose:
        v, ok := c.calls[p.id]
        if !ok {
          log.Infof("data close came in before header")
          return
        }
        v.Data.Write(tlv.Value)

        delete(c.calls, p.id)

        go func(call *Call){
          h, ok := c.handlers[call.Handler]
          if !ok {
            log.Infof("Handler %q not found", call.Handler)
            return
          }
          resp := h(call)
          resp.ID = call.ID // Adds the corresponding ID.
          c.responses <-h(call)
        }(v)
      }
    }
  }
}
