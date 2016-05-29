package local

import (
  "io/ioutil"
  "os"
  "testing"

  "github.com/johnsiilver/golib/filewatcher"
)

func setupFile() *os.File{
  f, err := ioutil.TempFile("", "filewatcher_tmp")
  if err != nil {
    panic(err)
  }

  if _, err := f.WriteString("hello"); err != nil {
    panic(err)
  }

  if err := f.Sync(); err != nil {
    panic(err)
  }

  return f
}

func Test(t *testing.T) {
  f := setupFile()
  defer os.Remove(f.Name())

  ch, closer, err := filewatcher.Get(filewatcher.Local+f.Name(), nil)
  if err != nil {
    t.Fatal(err)
  }
  defer closer()

  got := string(<-ch)
  if got != "hello" {
    t.Fatalf("file content: got %q, want %q", got, "hello")
  }

  if _, err := f.WriteString(" world"); err != nil {
    t.Fatal(err)
  }

  if err := f.Sync(); err != nil {
    panic(err)
  }

  got = string(<-ch)
  if got != "hello world" {
    t.Fatalf("file content: got %q, want %q", got, "hello world")
  }
}
