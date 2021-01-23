// +build linux

package uds

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// readCreds returns the credentials of the process calling the server.
// Taken from: https://blog.jbowen.dev/2019/09/using-so_peercred-in-go/
// Ref: // https://docs.fedoraproject.org/en-US/Fedora_Security_Team/1/html/Defensive_Coding/sect-Defensive_Coding-Authentication-UNIX_Domain.html
func readCreds(conn *net.UnixConn) (Cred, error) {
	var cred *unix.Ucred

	// Fetches raw network connection from UnixConn
	raw, err := conn.SyscallConn()
	if err != nil {
		return Cred{}, fmt.Errorf("error opening raw connection: %s", err)
	}

	var credErr error
	err = raw.Control(
		func(fd uintptr) {
			cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		},
	)

	if err != nil {
		return Cred{}, fmt.Errorf("Control() error: %s", err)
	}

	if credErr != nil {
		return Cred{}, fmt.Errorf("GetsockoptUcred() error: %s", credErr)
	}

	return Cred{PID: ID(cred.Pid), UID: ID(cred.Uid), GID: ID(cred.Gid)}, nil
}
