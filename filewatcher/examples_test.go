package filewatcher

import (
  "fmt"
)

func Example() {
  // Grab the file in the local file system at "path/to/file".
  // The "Local" marker indicates to use the local filesystem handler to fetch the file.
  // Remember you need to side-effect import in main: "github.com/johnsiilver/golib/filewatcher/local".
  ch, closer, err := Get(Local+"path/to/file", nil)
  if err != nil {
    // Do something
  }
  defer closer()

  // This fetches the initial content of the file.
  content := <-ch
  fmt.Println("Initial content of the file:")
  fmt.Println(string(content))

  // This fetches the content at each change.
  for b := range <-ch {
      fmt.Println("New content of the file:")
      fmt.Println(string(b))
  }
}
