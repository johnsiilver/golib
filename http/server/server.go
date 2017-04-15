/*
Package server provides a webserver constructor.

To use this, you must also use the registry package to register handlers.
That is generally done in modules by doing the following:

  func init() {
    registry.Register("/hello", handler)
  }

handler can be an existing http.Handler or can be made by using an

  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request{})

You can start the server in a few ways:

  // Start the server on the :http port (80).
  s, err := New()
  if err != nil {
    // Do something
  }
  s.ListenAndServe()  // This blocks.

  // Start on any free port.
  l, err := net.Listen("tcp", ":0") // Start listening on any free port.
	if err != nil {
    // Do something
  }
  fmt.Println("Server listening on: %s", l.Addr().String())

  s, err := New()
  if err != nil {
    // Do something
  }
  s.Serve(l)  // This blocks.

  // Start on a specific port.
  s, err := New(Addr(":2560"))
  if err != nil {
    // Do something
  }
  s.ListenAndServe()  // This blocks.
*/
package server

import (
	"crypto/tls"
	"log"
	"net/http"

	"github.com/johnsiilver/golib/http/server/registry"
)

// Option is an optional argument to the New() constructor.
type Option func(s *http.Server)

// TLS passes a TLS config to the server to use.
func TLS(t *tls.Config) Option {
	return func(s *http.Server) {
		s.TLSConfig = t
	}
}

// ErrorLog allows you to specify the errror log for the server.
// If not set, will use os.Stderr.
func ErrorLog(l *log.Logger) Option {
	return func(s *http.Server) {
		s.ErrorLog = l
	}
}

// Addr sets up the address for the server to listen on. By default this is
// :http.
func Addr(addr string) Option {
	return func(s *http.Server) {
		s.Addr = addr
	}
}

// ServeDir updates the server to serve files at fileDir path at at URL
// pattern.  So this might look like:
// ServeDir(server, "/img/", "images/")
// You should only call this before staring the server.
func ServeDir(serv *http.Server, pattern string, fileDir string) {
	serv.Handler.(*http.ServeMux).Handle(pattern, http.StripPrefix(pattern, http.FileServer(http.Dir(fileDir))))
}

// New is the constructor for an *http.Server.
func New(options ...Option) (*http.Server, error) {
	mux := http.NewServeMux()
	for k, v := range registry.Registry {
		mux.Handle(k, v)
	}

	s := &http.Server{Handler: mux}
	for _, opt := range options {
		opt(s)
	}
	return s, nil
}
