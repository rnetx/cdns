//go:build windows

package control

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	IP_UNICAST_IF   = 31
	IPV6_UNICAST_IF = 31
)

func bindToInterface(conn syscall.RawConn, isIPv6 bool, interfaceName string, interfaceIndex int) error {
	var inErr error
	err := conn.Control(func(fd uintptr) {
		handle := syscall.Handle(fd)
		if isIPv6 {
			inErr = syscall.SetsockoptInt(handle, syscall.IPPROTO_IPV6, IPV6_UNICAST_IF, interfaceIndex)
		} else {
			var bytes [4]byte
			binary.BigEndian.PutUint32(bytes[:], uint32(interfaceIndex))
			idx := *(*uint32)(unsafe.Pointer(&bytes[0]))
			inErr = syscall.SetsockoptInt(handle, syscall.IPPROTO_IP, IP_UNICAST_IF, int(idx))
		}
	})
	if inErr != nil {
		if err != nil {
			return fmt.Errorf("errors: %s, and %s", inErr, err)
		}
		return inErr
	}
	return err
}
