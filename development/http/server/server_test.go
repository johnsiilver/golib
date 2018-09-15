package server

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/johnsiilver/golib/development/http/server/registry"
)

func TestServer(t *testing.T) {
	const helloBody = "Hello"

	hello := func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, helloBody)
	}

	registry.Register("/hello", http.HandlerFunc(hello))

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("could not get a port to listen on: %s", err)
	}
	defer l.Close()

	addr := l.Addr().String()

	s, err := New()
	if err != nil {
		t.Fatalf("could not create http server: %s", err)
	}

	ServeDir(s, "/test_files/", "test_files/")

	go s.Serve(l)

	time.Sleep(5 * time.Second) // Yeah, this is lazy.

	str, err := fetchBody(fmt.Sprintf("http://%s/hello", addr))
	if err != nil {
		t.Fatalf("could not get /hello: %s", err)
	}

	if str != helloBody {
		t.Fatalf("/hello: got %s, want %s", str, helloBody)
	}

	str, err = fetchBody(fmt.Sprintf("http://%s/test_files/file1", addr))
	if err != nil {
		t.Fatalf("could not get /test_files/file1: %s", err)
	}

	if strings.TrimSpace(str) != "file1" {
		t.Fatalf("/test_files/file1: got %q, want %q", str, "file1")
	}
}

func fetchBody(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
