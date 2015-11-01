/*
tailor is a very simplistic program for replacing the key and value types in lru.go with concrete types that do not require
any type of runtime reflection. If this is a custom type, you will need to add the imports manually.
*/
package main

import (
  "bytes"
  "flag"
  "fmt"
  "io/ioutil"
  "os"
)

var (
  keyType = flag.String("key_type", "", "The type to replace key type with. Should include '*' if a pointer.")
  valType = flag.String("value_type", "", "The type to replace the value type with. Should include '*' if a pointer.")
)

func main() {
  flag.Parse()

  b, err := ioutil.ReadFile("../lru.go")
  if err != nil {
    fmt.Println("Error: could not locate lru.go . This program must run from the lru/tailor directory.")
    os.Exit(1)
  }

  for _, line := range bytes.Split(b, []byte("\n")) {
    line = bytes.Replace(line, []byte(`k interface{}`), []byte(fmt.Sprintf("k %s", *keyType)), -1)
    line = bytes.Replace(line, []byte(`v interface{}`), []byte(fmt.Sprintf("v %s", *valType)), -1)
    fmt.Println(string(line))
  }
}
