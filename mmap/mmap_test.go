package mmap

import (
  "io"
  "io/ioutil"
  "os"
  "path"
  "testing"

  "log"
)

func TestMap(t *testing.T) {
  tests := []struct{
    desc string
    opts []Option
    anon bool
    content string
    file string
    changes []byte
    err bool
  }{
    {
      desc: "Error: no Prots",
      opts: []Option{Flag(Shared)},
      err: true,
    },
    {
      desc: "Error: no Flag",
      opts: []Option{Prot(Read)},
      err: true,
    },
    {
      desc: "Error: Anon() without Length()",
      opts: []Option{Prot(Read), Flag(Private), Anon()},
      anon: true,
      err: true,
    },
    {
      desc: "Error: cannot map 0 length file",
      opts: []Option{Prot(Read), Flag(Shared)},
      content: "",
      err: true,
    },
    {
      desc: "Success full file read only shared",
      opts: []Option{Prot(Read), Flag(Shared)},
      content: "apples and oranges",
      file: "apples and oranges",
    },
    {
      desc: "Success full file read/write shared",
      opts: []Option{Prot(Read), Prot(Write), Flag(Shared)},
      content: "apples and oranges",
      changes: []byte("nectar"),
      file: "nectar and oranges",
    },
    {
      desc: "Success full file read/write private",
      opts: []Option{Prot(Read), Prot(Write), Flag(Private)},
      content: "apples and oranges",
      changes: []byte("nectar"),
      file: "apples and oranges",
    },
    {
      desc: "Success full file read/write shared/anon",
      opts: []Option{Prot(Read), Prot(Write), Flag(Private), Anon(), Length(8)},
      anon: true,
      content: "\x00\x00\x00\x00\x00\x00\x00\x00",
      file: "apples and oranges",
    },
  }

  for _, test := range tests {
    var f *os.File
    var name string
    if !test.anon {
      var err error
      f, err = ioutil.TempFile("", "")
      if err != nil {
             panic(err)
      }

      _, err = io.WriteString(f, test.content)
      if err != nil {
             panic(err)
      }

      s, err := f.Stat()
      if err != nil {
             panic(err)
      }
      name = s.Name()
    }

    m, err := NewMap(f, test.opts...)
    switch {
    case err == nil && test.err:
      t.Errorf("Test %q: got err == nil, want err != nil", test.desc)
      continue
    case err != nil && !test.err:
      t.Errorf("Test %q: got err == %q, want err == nil", test.desc, err)
      continue
    case err != nil:
      continue
    }

    var fPath string
    if !test.anon {
      fPath = path.Join(os.TempDir(), name)
    }

    if string(m.Bytes()) != test.content {
      t.Errorf("Test %q: got %q, want %q", test.desc, string(m.Bytes()), test.content)
    }

    if len(test.changes) != 0 {
      copy(m.Bytes(), test.changes)
    }

    if err := m.Close(); err != nil {
      t.Fatal(err)
    }

    if !test.anon {
      b, err := ioutil.ReadFile(fPath)
      if err != nil {
        t.Errorf("ioutil could not read the mapped file: %q", err)
        continue
      }

      if string(b) != test.file {
        t.Errorf("Test %q: got %q, want %q", test.desc, string(b), test.file)
      }
    }
  }
}

func TestReadWrite(t *testing.T) {
  const str = "apples and oranges"
  f, err := ioutil.TempFile("", "")
  if err != nil {
         panic(err)
  }

  _, err = io.WriteString(f, str)
  if err != nil {
         panic(err)
  }

  s, err := f.Stat()
  if err != nil {
         panic(err)
  }
  fPath := path.Join(os.TempDir(), s.Name())
  t.Log(fPath)

  m, err := NewMap(f, Prot(Read), Prot(Write), Flag(Shared))
  if err != nil {
    panic(err)
  }

  b := make([]byte, len(str))
  n, err := m.Read(b)
  if n == 0 && err != nil {
    t.Errorf("Read(): got n == 0, want n!=0")
  }else{
    if string(b) != str {
      t.Errorf("Read(): got %q, want %q", string(b), str)
    }
    if n != len(str) {
      t.Errorf("Read(): got n == %d, want %d", n, len(str))
    }

    if m.(*mmap).ptr != len(str) {
      t.Errorf("Read(): mmap.ptr, got %d, want %d", m.(*mmap).ptr, len(str))
    }
  }

  see, err := m.Seek(0, 0)
  switch {
  case err != nil:
    t.Fatalf("Seek(): got err == %q, want err == nil", err)
  case see != 0:
    t.Fatalf("Seek(): got n == %d, want n == 0", n)
  case m.(*mmap).ptr != 0:
    t.Fatalf("Seek(): got m.ptr == %d, want m.ptr == 0", m.(*mmap).ptr)
  }

  n, err = m.ReadAt(b, 1)
  if n == 0 || err == nil {
    t.Errorf("ReadAt(): got n == %d, err == %q, want n == %d, err != nil", n, err, len(str) -1)
  }

  if string(b[:len(b)-1]) != str[1:] {
    t.Errorf("ReadAt(): got %q, want %q", string(b), str[1:])
  }

  // ReadAt should not affect the internal pointer.
  if m.(*mmap).ptr != 0 {
    t.Errorf("ReadAt(): changed the internal pointer")
  }

  up := []byte("nectar")
  n, err = m.Write(up)
  if err != nil {
    t.Fatalf("Write(): got err == %q, want err == nil", err)
  }

  if n != len(up) {
    t.Errorf("Write(): got n == %d, want n == %d", n, len(up))
  }

  if m.(*mmap).ptr != len(up) {
    t.Fatalf("Write(): ptr did not advance")
  }

  m.Seek(0, 0)
  b = make([]byte, len(up))
  m.Read(b)
  if string(b) != string(up) {
    t.Fatalf("Write(): got %q, want %q", string(b), string(up))
  }
}

func TestString(t *testing.T) {
  const (
    str0 = "apples and oranges\n"
    str1 = "kicking it"
    replace = "kicking ti"
  )

  f, err := ioutil.TempFile("", "")
  if err != nil {
         panic(err)
  }

  _, err = io.WriteString(f, str0+str1)
  if err != nil {
         panic(err)
  }

  s, err := f.Stat()
  if err != nil {
         panic(err)
  }
  fPath := path.Join(os.TempDir(), s.Name())
  t.Log(fPath)

  m, err := NewString(f, Prot(Read), Prot(Write), Flag(Shared))
  if err != nil {
    panic(err)
  }

  // Test we read the first line.
  str, err := m.ReadLine()
  if err != nil {
    t.Fatalf("got err == %q, want err == nil", err)
  }

  if m.(*stringer).ptr != len(str0) {
    t.Fatalf("ReadLine() did move pointer to correct position, got %d want %d", m.(*stringer).ptr, len(str0))
  }

  if str != str0[:len(str0)-1] {
    t.Fatalf("got %q, want %q", str, str0)
  }

  // Test we read the second line.
  str, err = m.ReadLine()
  if err != io.EOF {
    t.Fatalf("got err == %q, want err == nil", err, io.EOF)
  }

  log.Println(str)

  if m.(*stringer).ptr != len(str0) + len(str1) {
    t.Fatalf("ReadLine() did move pointer to correct position, got %d want %d", m.(*stringer).ptr, len(str0) + len(str1))
  }

  if str != str1[:len(str1)] {
    t.Fatalf("got %q, want %q", str, str1)
  }

  // Make sure it fails if we try to write over the buffer.
  m.Seek(0,0)
  m.Seek(int64(m.Len()-len(replace)), 0)

  n, err := m.WriteString(replace)
  if err != nil {
    t.Fatalf("Got err == %q, want err == nil", err)
  }

  if n != len(replace) {
    t.Fatalf("WriteString() n value: got %d, want %d", n, len(replace))
  }

  if m.(*stringer).ptr != len(str0 + replace) {
    t.Fatalf("WriteString() ptr: got %d, want %d", m.(*stringer).ptr, len(str0+replace))
  }

}
