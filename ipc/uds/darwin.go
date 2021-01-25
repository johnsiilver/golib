// +build darwin

package uds

import (
	"fmt"
	"net"
	"os/user"
	"strconv"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/unix"
)

const (
	syscall_SOL_LOCAL     = 0
	syscall_LOCAL_PEERPID = 2
)

// readCreds returns the credentials of the process calling the server.
// Some of this came from: https://github.com/mysteriumnetwork/node/issues/2204
// So credit to: Andžej Maciusovič - if you read this, Labas!
func readCreds(conn *net.UnixConn) (Cred, error) {
	var pid int

	// Fetches raw network connection from UnixConn
	raw, err := conn.SyscallConn()
	if err != nil {
		return Cred{}, fmt.Errorf("error opening raw connection: %s", err)
	}

	var pidErr error
	err = raw.Control(
		func(fd uintptr) {
			pid, pidErr = unix.GetsockoptInt(int(fd), syscall_SOL_LOCAL, syscall_LOCAL_PEERPID)
		},
	)

	if err != nil {
		return Cred{}, fmt.Errorf("Control() error: %s", err)
	}

	if pidErr != nil {
		return Cred{}, fmt.Errorf("GetsockoptInt() error: %s", pidErr)
	}

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return Cred{}, fmt.Errorf("could not find PID for client connecting to socket: %w", err)
	}

	uids, err := proc.Uids()
	if err != nil || len(uids) == 0 {
		return Cred{}, fmt.Errorf("could not find UIDs associated with client's PID(%v): %w", pid, err)
	}

	u, err := user.LookupId(strconv.Itoa(int(uids[0])))
	if err != nil {
		return Cred{}, fmt.Errorf("could not lookup UID(%v) for client PID(%v): %w", uids[0], pid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Cred{}, fmt.Errorf("could not lookup GID for UID(%v) PID(%v): %w", uids[0], pid, err)
	}

	return Cred{PID: ID(pid), UID: ID(uids[0]), GID: ID(gid)}, nil
}
