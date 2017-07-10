/*
Package signal provides a generic signaling package for communication between
two goroutines for bilatteral communication.

To answer a few questions right off the bat:

1. This can be done more effectively with type safety.

2. There is a minor slowdown due to interface{}.

This package provides the boilerplate for signaling between goroutines,
usually done by passing an object across a channel with a return value channel
included, like:

	type signal struct {
		value <type>
		response chan <type>
	}

	ch := make(chan signal, 1)
	go receiver(ch)

	sig := signal{value: <value>, resp: make(chan <type>, 1)}
	ch <- sig
	...
	resp := <-sig.response

And it solves a few other boilerplate code you may be tired of writing.  If
speed or safety become an issue, it is easy to substitute out for concrete
structures similar to what you see above.

Example using Wait() and receiving data:
	sig := signal.New()

	go func(sig Signaler) {
		ack := <-sig.Receive()
		defer ack.Ack("I'm out!") // Acknowledge receipt and return data.

		// This will print "hello everyone".
		fmt.Println(<-ack.Date().(string))
	}(sig)

	// This will wait until ack.Ack() is called above.
	retData := sig.Signal("hello everyone", signal.Wait()).(string)

	// This will print "I'm out".
	fmt.Println(retData)

Example using promises for asynchronous return values:
	sig := signal.New()

	go func(sig Signaler) {
		ack := sig.Receive()
		defer ack.Ack("done")

		fmt.Println("go here")
	}(sig)

	p := make(chan interface{}, 1)
	sig.Signal(nil, sigmal.Promise(p))
	...
	// Do some other stuff
	...
	fmt.Println((<-p).(string)) // Prints "I'm out".
}
*/
package signal

// Acker provides the ability to acknowledge a Signal.
type Acker struct {
	data interface{}
	ack  chan interface{}
}

// Data returns any data sent by the sender.
func (a Acker) Data() interface{} {
	return a.data
}

// Ack acknowledges a Signal has been received. "x" is any data you wish to return.
func (a Acker) Ack(x interface{}) {
	a.ack <- x
	close(a.ack)
}

// SignalOption provides an option to Signaler.Signal()
type SignalOption func(s Signaler) Signaler

// Wait indicates that Signal() should block until the Acker has had Ack() called.
func Wait() SignalOption {
	return func(s Signaler) Signaler {
		s.wait = true
		return s
	}
}

// Promise can be used to send a signal without waiting for the data to be
// returned, but still get the data at a later point.
// Using Promise() and Wait() will PANIC.
// Passing Promise() a nil pointer will PANIC.
func Promise(ch chan interface{}) SignalOption {
	return func(s Signaler) Signaler {
		if ch == nil {
			panic("you cannot use a nil channel with Promise()")
		}
		s.promise = ch
		return s
	}
}

// Option is an option for the New() constructor.
type Option func(s Signaler) Signaler

// BufferSize lets you adjust the internal buffer for how many Signal() calls
// you can make before Signal() blocks waiting someone to call Receive().
func BufferSize(n int) Option {
	return func(s Signaler) Signaler {
		s.bufferSize = n
		return s
	}
}

// Signaler provides an object that can be passed to other goroutines to
// provide for a signal that something has happened.  The receiving goroutine
// can call Receive(), which will block until Signal() is called.
type Signaler struct {
	sendCh     chan Acker
	bufferSize int

	wait    bool
	promise chan interface{}
}

// New is the constructor for Signal.
func New(options ...Option) Signaler {
	s := Signaler{bufferSize: 1}
	for _, o := range options {
		s = o(s)
	}
	s.sendCh = make(chan Acker, s.bufferSize)
	return s
}

// Signal signals the goroutine calling Receive().  This unblocks the
// Receive call. The return value is data returned by the acknowledger, which
// may be nil.
func (s Signaler) Signal(x interface{}, options ...SignalOption) interface{} {
	for _, option := range options {
		s = option(s)
	}
	if s.promise != nil && s.wait == true {
		panic("Signaler.Signal() cannot be called with both Wait() and Promise()")
	}

	a := Acker{
		data: x,
		ack:  make(chan interface{}, 1),
	}

	s.sendCh <- a

	if s.wait {
		return <-a.ack
	}

	if s.promise != nil {
		go func() {
			s.promise <- <-a.ack
		}()
	}
	return nil
}

// Receive is used by the waiting goroutine to block until Signal() is
// called. The receiver should use the provided Acker.Ack() to inform
// Signal that it can continue (if it is using the Wait() option).
func (s Signaler) Receive() <-chan Acker {
	return s.sendCh
}

// Close closes all the internal channels for Signaler. This will stop any
// for range loops over the .Receive() channel. This Signaler cannot be used again.
func (s Signaler) Close() {
	close(s.sendCh)
}
