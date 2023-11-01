//go:build linux

package control

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func bindToInterface(conn syscall.RawConn, _ bool, interfaceName string, interfaceIndex int) error {
	var inErr error
	err := conn.Control(func(fd uintptr) {
		inErr = unix.BindToDevice(int(fd), interfaceName)
	})
	if inErr != nil {
		if err != nil {
			return fmt.Errorf("errors: %s, and %s", inErr, err)
		}
		return inErr
	}
	return err
}
