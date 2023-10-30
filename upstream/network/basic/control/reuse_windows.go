//go:build windows

package control

import (
	"fmt"
	"syscall"
)

func ReuseAddr() func(network, address string, conn syscall.RawConn) error {
	return func(network, address string, conn syscall.RawConn) error {
		var innerErr error
		err := conn.Control(func(fd uintptr) {
			innerErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
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
