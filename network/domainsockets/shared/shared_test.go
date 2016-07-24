package shared

import (
  "bytes"
  "testing"

  "github.com/kylelemons/godebug/pretty"
  "github.com/pborman/uuid"
)

func TestSetupEncodeDecode(t *testing.T) {
    buff := new(bytes.Buffer)
    want := uuid.New()
    SetupEncode(want, buff)
    got, err := SetupDecode(buff)
    if err != nil {
      t.Fatal(err)
    }

    if got != want {
      t.Errorf("got %q, want %q", got, want)
    }
}

func TestClientMsg(t *testing.T) {
  msg := ClientMsg{
    ID: 32,
    Data: []byte("hello world"),
    Handler: "test",
  }

  buff := new(bytes.Buffer)
  if err := msg.Encode(buff); err != nil {
    t.Fatalf("problem encoding message: %s", err)
  }

  got := ClientMsg{}
  if err := got.Decode(buff); err != nil {
    t.Fatalf("could not decode the encoded message: %s", err)
  }

  if diff := pretty.Compare(msg, got); diff != "" {
    t.Fatalf("-want/+got:\n%s", diff)
  }
}

func TestServerMsg(t *testing.T) {
  msg := ServerMsg{
    ID: 32,
    Type: 1,
    Data: []byte("test"),
  }

  buff := new(bytes.Buffer)
  if err := msg.Encode(buff); err != nil {
    t.Fatalf("problem encoding message: %s", err)
  }

  got := ServerMsg{}
  if err := got.Decode(buff); err != nil {
    t.Fatalf("could not decode the encoded message: %s", err)
  }

  if diff := pretty.Compare(msg, got); diff != "" {
    t.Fatalf("-want/+got:\n%s", diff)
  }
}
