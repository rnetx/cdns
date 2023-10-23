//go:build linux

package control

import (
	"fmt"
	"syscall"
)

func SetMark(mark uint32) func(network string, address string, c syscall.RawConn) error {
	return func(network string, address string, c syscall.RawConn) error {
		var inErr error
		err := c.Control(func(fd uintptr) {
			inErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, int(mark))
		})
		if inErr != nil {
			if err == nil {
				err = inErr
			}
			return fmt.Errorf("errors: %s, and %s", err, inErr)
		}
		return err
	}
}
