package rpc

import "fmt"

// ErrType indicates the type of message to use.
type ErrType int8

const (
	// ETUnknown means to use the ErrUnknown type.
	ETUnknown = 0
	// ETMethodNotFound means to use the ErrMethodNotFound type.
	ETMethodNotFound = 1
	// ETDeadlineExceeded means to use the ErrDeadlineExceeded type.
	ETDeadlineExceeded = 2
	// ETBadData means to use the ErrBadData type.
	ETBadData = 3
	// ETServer means to use the ErrServer type.
	ETServer = 4
)

type setter interface {
	set(msg string) error
}

type retryer interface {
	retry()
}

// ErrUnknown indicates the error type is unknown.
type ErrUnknown struct {
	msg string
}

func (e ErrUnknown) Error() string {
	return e.msg
}

func (e ErrUnknown) set(msg string) error {
	return ErrUnknown{msg: msg}
}

// ErrMethodNotFound indicates that the calling method wasn't found on the server.
type ErrMethodNotFound struct {
	msg string
}

func (e ErrMethodNotFound) Error() string {
	return e.msg
}

func (e ErrMethodNotFound) set(msg string) error {
	return ErrMethodNotFound{msg: msg}
}

// ErrDeadlineExceeded indicates call deadline was exceeded.
type ErrDeadlineExceeded struct {
	msg string
}

func (e ErrDeadlineExceeded) retry() {}

func (e ErrDeadlineExceeded) Error() string {
	return e.msg
}

func (e ErrDeadlineExceeded) set(msg string) error {
	return ErrDeadlineExceeded{msg: msg}
}

// ErrBadData indicates that client sent bad data according to the server.
type ErrBadData struct {
	msg string
}

func (e ErrBadData) Error() string {
	return e.msg
}

func (e ErrBadData) set(msg string) error {
	return ErrBadData{msg: msg}
}

// ErrServer indicates the server sent back an error message.
type ErrServer struct {
	msg string
}

func (e ErrServer) Error() string {
	return e.msg
}

func (e ErrServer) set(msg string) error {
	return ErrServer{msg: msg}
}

var codeToErr = []setter{
	ErrUnknown{},
	ErrMethodNotFound{},
	ErrDeadlineExceeded{},
	ErrBadData{},
	ErrServer{},
}

// Errorf returns an error specificed by the ErrType containing the error text
// of fmt.Sprintf(s, i...).
func Errorf(code ErrType, s string, i ...interface{}) error {
	err := codeToErr[int(code)]
	if err == nil {
		return fmt.Errorf(s, i...)
	}
	return err.set(fmt.Sprintf(s, i...))
}

// Retryable indicates that the error is retryable.
func Retryable(err error) bool {
	if _, ok := err.(retryer); ok {
		return true
	}
	return false
}
