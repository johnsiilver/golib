/*
Package portpicker provides an available TCP or UDP port to use in a service above port 1023. It does not guarentee
that the port will remain available.

Get a TCP port to bind on all valid IPs for binding:
	port, err := TCP()
	if err != nil {
		panic(err)
	}

Same, but panic if it can't be done:
	port := Must(TCP)

Get a UDP port on IP 192.168.0.1 and the IPV6 localhost:
	port := Must(UDP, IP(net.ParseIP("192.168.0.1"), Local6()))

Get a TCP port on an IPv4 localhost:
	port := Must(TCP, Local4())

When binding to localhost, use "localhost:<port>", don't assume [::1]:<port> or 127.0.0.1:<port.
If you want that, use option IP("127.0.0.1") instead.
*/
package portpicker

import (
	"fmt"
	"net"
)

type opts struct {
	addr        net.IP
	localHostV4 bool
	localHostV6 bool
}

func (o opts) isZero() bool {
	if o.addr == nil && !o.localHostV4 && !o.localHostV6 {
		return true
	}
	return false
}

func (o opts) validate() error {
	if o.addr != nil {
		if o.addr.String() == "" {
			return fmt.Errorf("IP passed was not a valid IP")
		}
	}
	return nil
}

// Option is an optional argument.
type Option func(o *opts)

// IP specifies you want to specify a specific host address to bind to.
func IP(ip net.IP) Option {
	return func(o *opts) {
		o.addr = ip
	}
}

// Local4 informs we want to bind to the IPv4 localhost address.
func Local4() Option {
	return func(o *opts) {
		o.localHostV4 = true
	}
}

// Local6 informs we want to bind to the IPv6 localhost address.
func Local6() Option {
	return func(o *opts) {
		o.localHostV6 = true
	}
}

// TCP returns a TCP port that can be bound to. With no options passed it will bind to
// all IPs on the system. If an option is passed, it will only bind to the options specified.
func TCP(options ...Option) (int, error) {
	optsv := opts{}
	for _, o := range options {
		o(&optsv)
	}

	if err := optsv.validate(); err != nil {
		return -1, err
	}

	if optsv.isZero() {
		addr, err := net.ResolveTCPAddr("tcp", ":0")
		if err != nil {
			return -1, err
		}
		return addr.Port, nil
	}

	for i := 1024; i < 65536; i++ {
		if optsv.addr != nil {
			if !canListenTCP("tcp", optsv.addr.String(), i) {
				continue
			}
		}
		if optsv.localHostV4 {
			if !canListenTCP("tcp4", "localhost", i) {
				continue
			}
		}
		if optsv.localHostV6 {
			if !canListenTCP("tcp6", "localhost", i) {
				continue
			}
		}
		return i, nil
	}

	return -1, fmt.Errorf("could not find any open ports matching the criteria")
}

// UDP returns a UDP port that can be bound to. With no options passed it will bind to
// all IPs on the system. If an option is passed, it will only bind to the options specified.
func UDP(options ...Option) (int, error) {
	optsv := opts{}
	for _, o := range options {
		o(&optsv)
	}

	if err := optsv.validate(); err != nil {
		return -1, err
	}

	if optsv.isZero() {
		addr, err := net.ResolveUDPAddr("udp", ":0")
		if err != nil {
			return -1, err
		}
		return addr.Port, nil
	}

	for i := 1; i < 65536; i++ {
		if optsv.addr != nil {
			if !canListenUDP("udp", optsv.addr.String(), i) {
				continue
			}
		}
		if optsv.localHostV4 {
			if !canListenUDP("udp4", "localhost", i) {
				continue
			}
		}
		if optsv.localHostV6 {
			if !canListenUDP("udp6", "localhost", i) {
				continue
			}
		}
		return i, nil
	}

	return -1, fmt.Errorf("could not find any open ports matching the criteria")
}

func canListenTCP(t string, ip string, port int) bool {
	if t == "tcp6" && ip == "localhost" {
		addr, err := net.ResolveTCPAddr(t, fmt.Sprintf("[::1]:%d", port))
		if err != nil {
			return false
		}
		l, err := net.ListenTCP(t, addr)
		if err != nil {
			return false
		}
		l.Close()
		return true
	}
	addr, err := net.ResolveTCPAddr(t, fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return false
	}
	l, err := net.ListenTCP(t, addr)
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func canListenUDP(t string, ip string, port int) bool {
	addr, err := net.ResolveUDPAddr(t, fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return false
	}
	l, err := net.ListenUDP(t, addr)
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// Must is used to call either a passed TCP() or UDP() call and its options. If there is
// an error, it panics
func Must(f func(options ...Option) (int, error), options ...Option) int {
	p, err := f(options...)
	if err != nil {
		panic(err)
	}
	return p
}
