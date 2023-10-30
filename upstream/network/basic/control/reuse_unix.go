//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package control

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func ReuseAddr() func(network, address string, conn syscall.RawConn) error {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			if innerErr != nil {
				return
			}
			innerErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		})
		if innerErr != nil {
			if err != nil {
				return fmt.Errorf("%w | %w", err, innerErr)
			}
			return innerErr
		}
		return err
	}
}
