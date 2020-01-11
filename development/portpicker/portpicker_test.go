package portpicker

import (
	"net"
	"testing"
)

func ips() []net.IP {
	ips := []net.IP{}
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			switch false {
			case ip.IsGlobalUnicast():
				continue
			}
			ips = append(ips, ip)
		}
	}
	return ips
}

func TestTCP(t *testing.T) {
	list := ips()
	if len(list) == 0 {
		panic("can't run test wihtout some non-loopback and non-multicast IPs")
	}

	port := Must(TCP, IP(list[0]), Local4(), Local6())

	// Is the IP:port open?
	if !canListenTCP("tcp", list[0].String(), port) {
		t.Fatalf("TestTCP got port %d, but we can't listen to it on IP %v", port, list[0])
	}

	// Is IPv4 localhost:port open?
	addr, err := net.ResolveTCPAddr("tcp4", "localhost:0")
	if err != nil {
		panic(err)
	}
	if !canListenTCP("tcp4", addr.IP.String(), port) {
		t.Fatalf("TestTCP got port %d, but we can't listen to it on IP %v", port, addr.IP.String())
	}

	// Is IPv6 localhost:port open?
	addr, err = net.ResolveTCPAddr("tcp6", "localhost:0")
	if err != nil {
		panic(err)
	}

	if !canListenTCP("tcp6", "localhost", port) {
		t.Fatalf("TestTCP got port %d, but we can't listen to it on IP %v", port, addr.IP.String())
	}
}

func TestUDP(t *testing.T) {
	list := ips()
	if len(list) == 0 {
		panic("can't run test wihtout some non-loopback and non-multicast IPs")
	}

	port := Must(UDP, IP(list[0]), Local4(), Local6())

	// Is the IP:port open?
	if !canListenUDP("udp", list[0].String(), port) {
		t.Fatalf("TestUDP got port %d, but we can't listen to it on IP %v", port, list[0])
	}

	// Is IPv4 localhost:port open?
	addr, err := net.ResolveUDPAddr("udp4", "localhost:0")
	if err != nil {
		panic(err)
	}
	if !canListenUDP("udp4", addr.IP.String(), port) {
		t.Fatalf("TestUDP got port %d, but we can't listen to it on IP %v", port, addr.IP.String())
	}

	// Is IPv6 localhost:port open?
	addr, err = net.ResolveUDPAddr("udp6", "localhost:0")
	if err != nil {
		panic(err)
	}

	if !canListenUDP("udp6", "localhost", port) {
		t.Fatalf("TestUDP got port %d, but we can't listen to it on IP %v", port, addr.IP.String())
	}
}
