// Package registry contains a registry of URL patterns to http.Handlers
// and a Register() method for registering a handler during init() in a module.
package registry

import (
	"fmt"
	"net/http"
)

// Registry contains a mapping of URL patterns to http.Handler.
// This should never be modified except by Register() and only
// during init().
var Registry = map[string]http.Handler{}

// Register registers a pattern with a handler.
func Register(pattern string, handler http.Handler) {
	if _, ok := Registry[pattern]; ok {
		panic(fmt.Sprintf("Cannot register the pattern %q twice", pattern))
	}

	Registry[pattern] = handler
}
