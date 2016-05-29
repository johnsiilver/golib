/*
Package filewatcher provides facilities for watching a file on a filesystem for changes..

When To Use

This package is meant to provide abstraction for watching a single file, be it on a local filesystem or remote system.
This is useful for watching configuraiton files that may be local or stored on a remote service.
For watching multiple files on a local filesystem, use fsnotify.

Usage Note

All file paths must be prepended with a *marker* that indicates what type of filesystem the
file is on so it can be routed to the right handler.

For example:
  "local:/path/to/file"

SideEffect Imports

To use the filewatcher, you must side-effect import a filesystem implementation, such as "local" in your main.
*/
package filewatcher

import (
  "fmt"
  "strings"
)

const (
  // Local indicates the local filesystem, which was be done through standard Go libraries such as io and ioutil.
  Local = "local:"
)

// registry holds all the file markers to the type of watcher that can watch that file system.
var registry = map[string]func()Watch{}

// Register registers a filewatcher for files with "marker" using "watcher".
func Register(marker string, watcher func()Watch) {
  sp := strings.Split(marker, ":")
  if _, ok := registry[sp[0]]; ok {
    panic(fmt.Sprintf("filewatcher marker %q has already been registered", sp[0]))
  }

  registry[sp[0]] = watcher
}

// Get watches a file and sends changes on the returned channel. The file that exists when
// starting the watcher is streamed back as the first result. "f" must be prepended with a
// marker that indicates the file system that will be accessed.  "i" provides the ability to
// pass data structures to the underlying file access implementation (such as auth credentials or RPC endpoint information).
func Get(f string, i interface{}) (results chan []byte, closer func(), err error) {
  r := strings.Split(f, ":")
  if len(r) != 2 {
    return nil, nil, fmt.Errorf("could not locate the file system marker that should be prepended: %q", f)
  }

  v, ok := registry[r[0]]
  if !ok {
    return nil, nil, fmt.Errorf("could not locate an implementation that can read the file system designated by marker %q", r[0])
  }

  w := v()

  ch, err := w.File(r[1], i)
  if err != nil {
    return nil, nil, err
  }

  return ch, w.Close, nil
}

// Watcher watches a file and sends changes on the returned channel.
// The Watcher will return the current file when started.  Files always must
// be prepended with a marker indicating the type of file system we are
// accessing.
type Watch interface{
  // File watches the file at "f".  "f" WILL NOT contain the marker. The first channel
  // contains the file being streamed back.  "i" provides the ability to
  // pass data structures to the underlying file access implementation (such as auth credentials or RPC endpoint information).
  File(f string, i interface{}) (chan []byte, error)

  // Close stops the watcher from watching the file.
  Close()
}
