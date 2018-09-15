package local
// local.go implements a watcher for the local fileystem via fsnotify.

import (
  "fmt"
  "io/ioutil"
  "sync"

  "github.com/johnsiilver/golib/filewatcher"
  "github.com/fsnotify/fsnotify"

  log "github.com/golang/glog"
)

var dispatch *dispatcher

type dispatcher struct {
  watcher *fsnotify.Watcher
  routes map[string]chan struct{}
  closer chan struct{}
  count int
  sync.Mutex
}

func newDispatcher() (*dispatcher, error) {
  watcher, err := fsnotify.NewWatcher()
  if err != nil {
      return nil, err
  }
  d := &dispatcher{watcher: watcher, closer: make(chan struct{}), routes: map[string]chan struct{}{}}
  go d.listen()
  return d, nil
}

func (d *dispatcher) listen() {
  for {
      select {
      case event := <-d.watcher.Events:
        d.Lock()
        defer d.Unlock()
        d.routes[event.Name]<-struct{}{}
      case err := <-d.watcher.Errors:
          log.Errorf("problem with filewatcher: %s", err)
      case <-d.closer:
        return
      }
  }
}

func (d *dispatcher) close() {
  close(d.closer)
}

func (d *dispatcher) add(f string, route chan struct{}) error {
  d.Lock()
  defer d.Unlock()

  if _, ok := d.routes[f]; ok {
    return fmt.Errorf("file %q is already watched", f)
  }

  if err := d.watcher.Add(f); err != nil {
    return err
  }

  d.count++
  d.routes[f] = route
  return nil
}

func (d *dispatcher) remove(f string) error {
  d.Lock()
  defer d.Unlock()

  if _, ok := d.routes[f]; !ok {
    return fmt.Errorf("file %q was not being watched", f)
  }

  if err := d.watcher.Remove(f); err != nil {
    return err
  }
  d.count--
  close(d.routes[f])
  delete(d.routes, f)
  return nil
}

// local implements watcher for the local file system.
type local struct {
  file string
  running bool
  disUpdates chan struct{}
  fileContent chan []byte
  close, stopped chan struct{}
  sync.Mutex
}

// Watch implements filewatcher.Watcher.Watch().
func (l *local) File(f string, i interface{}) (chan []byte, error) {
  l.Lock()
  defer l.Unlock()

  if l.running {
    return nil, fmt.Errorf("file watcher already running")
  }

  l.file = f

  b, err := ioutil.ReadFile(f)
  if err != nil {
    return nil, fmt.Errorf("problem reading file %q: %s", f, err)
  }
  ch := make(chan []byte, 1)
  dCh := make(chan struct{}, 1)
  l.fileContent = ch
  l.disUpdates = dCh
  l.stopped = make(chan struct{})
  ch <- b

  if err := dispatch.add(f, l.disUpdates); err != nil {
    return nil, err
  }
  go l.listen()

  return ch, nil
}

func (l *local) listen() {
  for _ = range l.disUpdates {
    log.Infof("saw event")
    b, err := ioutil.ReadFile(l.file)
    if err != nil {
      log.Error(err)
      continue
    }
    l.fileContent <-b
  }
  close(l.stopped)
}

// Close implements filewatcher.Watcher.Close().
func (l *local) Close() {
  l.Lock()
  defer l.Unlock()
  l.running = false
  close(l.disUpdates)
  <-l.stopped
}

func init() {
  var err error
  dispatch, err = newDispatcher()
  if err != nil {
    panic(err)
  }

  filewatcher.Register(filewatcher.Local, func()filewatcher.Watch{return &local{}})
}
